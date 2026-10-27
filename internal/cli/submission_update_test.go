package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
	"github.com/hearthroom/cli/internal/card"
)

func updateSubmitArgs(t *testing.T, site *ratingSite) ([]string, string) {
	t.Helper()
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv := site.server(t)
	dir := ratingFolder(t, card.State{Target: "owned", RoleID: "role-1"})
	if err := card.WriteRating(dir, &card.Rating{Answers: card.RatingAnswers{Version: 1, Other: "other.none"}, Rating: "G"}); err != nil {
		t.Fatal(err)
	}
	return []string{"card", "submit", dir, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg}, dir
}

func TestSubmitUpdateContract(t *testing.T) {
	for _, tc := range []struct {
		name, note, want string
		major            bool
	}{
		{"default silent", "", "", false},
		{"empty silent note", "\r\n\u2003", "", false},
		{"normalize lines and trim", " \r\nNew route\r\nand ending\rhere\u2003", "New route\nand ending\nhere", true},
		{"10 code points despite surrogate pairs", strings.Repeat("😀", 10), strings.Repeat("😀", 10), true},
		{"200 code points", strings.Repeat("😀", 200), strings.Repeat("😀", 200), true},
		{"200 includes spaces and line breaks", "1234567890" + strings.Repeat(" \n", 94) + "ab", "1234567890" + strings.Repeat(" \n", 94) + "ab", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := &ratingSite{}
			args, _ := updateSubmitArgs(t, site)
			if tc.major {
				args = append(args, "--notify")
			}
			if tc.note != "" {
				args = append(args, "--update-note", tc.note)
			}
			stdout, stderr, err := runCLI(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			wantKind := "minor"
			if tc.major {
				wantKind = "major"
			}
			if len(site.submits) != 1 || site.submits[0]["updateKind"] != wantKind || site.submits[0]["updateNote"] != tc.want {
				t.Fatalf("submission = %+v", site.submits)
			}
			if tc.want != "" && (strings.Contains(stdout, tc.want) || strings.Contains(stderr, tc.want)) {
				t.Fatal("update note copied to terminal output")
			}
		})
	}
}

func TestSubmitUpdateRejectsInvalidNotesBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name, note, wantErr string
		major               bool
	}{
		{"missing", "", "An update notification needs a changelog. Use --update-note to describe what changed.", true},
		{"whitespace", "\n\t\u2003\u00a0", "An update notification needs a changelog. Use --update-note to describe what changed.", true},
		{"nine astral points", strings.Repeat("😀", 9), "Add details about what is new or changed to reach 10 characters, excluding whitespace.", true},
		{"spaces cannot make ten", "abc def ghi\u2003\n", "Add details about what is new or changed to reach 10 characters, excluding whitespace.", true},
		{"201 astral points", strings.Repeat("😀", 201), "Shorten the changelog to 200 characters or fewer before submitting.", true},
		{"201 includes whitespace", "1234567890" + strings.Repeat(" ", 190) + "a", "Shorten the changelog to 200 characters or fewer before submitting.", true},
		{"note does not authorize notification", "New route and ending", "A changelog requires Update and notify. Add --notify or remove --update-note before submitting.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := &ratingSite{}
			args, dir := updateSubmitArgs(t, site)
			if tc.major {
				args = append(args, "--notify")
			}
			args = append(args, "--update-note", tc.note)
			stdout, stderr, err := runCLI(t, args...)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("validation error = %v", err)
			}
			if len(site.submits) != 0 {
				t.Fatal("invalid note submitted")
			}
			f, loadErr := card.Load(dir)
			if loadErr != nil || f.State.Submission != nil {
				t.Fatalf("validation created pending operation: %+v %v", f, loadErr)
			}
			if strings.TrimSpace(tc.note) != "" && (strings.Contains(stdout, tc.note) || strings.Contains(stderr, tc.note) || strings.Contains(errString(err), tc.note)) {
				t.Fatal("invalid note copied to output")
			}
		})
	}
}

func TestSubmitUpdateStillRequiresAuthorAuthentication(t *testing.T) {
	site := &ratingSite{}
	args, _ := updateSubmitArgs(t, site)
	t.Setenv(auth.EnvToken, "")
	if _, _, err := runCLI(t, append(args, "--notify", "--update-note", "New route and ending")...); err == nil {
		t.Fatal("notifying update bypassed authentication")
	}
	if len(site.submits) != 0 {
		t.Fatal("unauthenticated request sent")
	}
}

func TestSubmitUpdateUsesNotifyFlag(t *testing.T) {
	_, _, err := runCLI(t, "card", "submit", "--major-update")
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --major-update") {
		t.Fatalf("retired draft flag is still accepted: %v", err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestSubmitUpdateRetryDigest(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second []string
		wantSame      bool
	}{
		{"same major", []string{"--notify", "--update-note", "New route and ending"}, []string{"--notify", "--update-note", "New route and ending"}, true},
		{"normalized note", []string{"--notify", "--update-note", " New route\r\nand ending "}, []string{"--notify", "--update-note", "New route\nand ending"}, true},
		{"changed note", []string{"--notify", "--update-note", "New route and ending"}, []string{"--notify", "--update-note", "Another route and ending"}, false},
		{"silent to notifying", nil, []string{"--notify", "--update-note", "New route and ending"}, false},
		{"notifying to silent", []string{"--notify", "--update-note", "New route and ending"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := &ratingSite{cardReplies: []int{http.StatusBadGateway, http.StatusCreated}}
			args, _ := updateSubmitArgs(t, site)
			if _, _, err := runCLI(t, append(append([]string{}, args...), tc.first...)...); err == nil {
				t.Fatal("first attempt should retain unknown operation")
			}
			if _, _, err := runCLI(t, append(append([]string{}, args...), tc.second...)...); err != nil {
				t.Fatal(err)
			}
			if len(site.submits) != 2 || (site.submits[0]["operationId"] == site.submits[1]["operationId"]) != tc.wantSame {
				t.Fatalf("operation identity wrong: %+v", site.submits)
			}
		})
	}
}

func TestSubmitUpdatePreservesLegacySilentOperation(t *testing.T) {
	site := &ratingSite{}
	args, dir := updateSubmitArgs(t, site)
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	rating, err := card.ReadRating(dir)
	if err != nil {
		t.Fatal(err)
	}
	legacyDigest := card.Digest(map[string]any{"roleId": f.State.RoleID, "answers": rating.Answers, "fandom": "", "sections": f.State.Sections})
	op := "11111111-2222-4333-8444-555555555555"
	f.State.Submission = &card.Submission{RoleID: f.State.RoleID, OperationID: op, Digest: legacyDigest}
	if err := f.SaveState(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if len(site.submits) != 1 || site.submits[0]["operationId"] != op {
		t.Fatalf("upgrade changed pending operation: %+v", site.submits)
	}
}
