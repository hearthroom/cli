package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/media"
	"github.com/hearthroom/cli/internal/sync"
)

// tinyPNG is a 1×1 transparent PNG.
var tinyPNG = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 0x49, 0x48, 0x44, 0x52, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89, 0, 0, 0, 0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0, 1, 0, 0, 5, 0, 1, 0x0d, 0x0a, 0x2d, 0xb4, 0, 0, 0, 0, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}

func fixtureFolder(t *testing.T) *card.Folder {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "e2e-card")
	f, err := card.Init(dir, "E2E Card")
	if err != nil {
		t.Fatal(err)
	}
	f.Manifest.Summary = "A card pushed by the end-to-end test."
	f.Manifest.Tags = []string{"test"}
	f.Manifest.Prologue = []string{"Begin."}
	f.Manifest.Media.Portrait = "assets/portrait.png"
	f.Definition = "E2E is a character that exists to verify the CLI. Map: assets/map.png"
	f.Welcome = "Hello from the end-to-end test."
	f.Alternates = []card.Opening{{Text: "Alternate hello."}}
	f.Lorebook = &card.Lorebook{Name: "E2E Lore", Entries: []card.LorebookEntry{
		{Name: "Harbor", Content: "A quiet harbor.", Keywords: []string{"harbor"}},
		{Name: "Lamp", Content: "A lamp.", Keywords: []string{"lamp"}},
	}}
	f.Rules = &card.Rules{Rules: []card.DisplayRule{{Find: "\\*\\*(.+?)\\*\\*", Replace: "<b>$1</b>", Enabled: true}}, MountLayer: "under"}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"portrait.png", "map.png"} {
		if err := os.WriteFile(filepath.Join(dir, "assets", name), tinyPNG, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestTrialPushValidateAndPull(t *testing.T) {
	e := setup(t)
	e.login(t)
	ctx := context.Background()
	f := fixtureFolder(t)

	caps, err := media.Probe(ctx, e.client)
	if err != nil {
		t.Fatal(err)
	}

	res, err := sync.Push(ctx, e.client, f, sync.PushOptions{Evict: true})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if res.Target != "trial" || res.RoleID == "" || !res.Created {
		t.Fatalf("push result: %+v", res)
	}
	if len(res.Changed) != 5 || res.Changed[4] != "media" {
		t.Fatalf("first push should send all four sections plus media, got %v", res.Changed)
	}
	uploaded := 0
	for _, a := range res.Assets {
		if a.Err != "" {
			t.Fatalf("asset %s: %s", a.Path, a.Err)
		}
		if a.Uploaded {
			uploaded++
		}
		if caps.RelativePaths && !strings.HasPrefix(a.URL, caps.LibraryPrefix) {
			t.Fatalf("asset %s served from %s, want prefix %s", a.Path, a.URL, caps.LibraryPrefix)
		}
	}
	if uploaded != 2 {
		t.Fatalf("uploaded %d assets, want 2", uploaded)
	}

	// The provider stored the portrait URL, not the relative path.
	var detail card.RemoteDetail
	if err := e.client.OpenGet(ctx, "/role/detail", map[string][]string{"roleId": {res.RoleID}}, &detail); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(detail.RoleAvatar, "http") || strings.Contains(detail.RoleDetailDesc, "assets/map.png") {
		t.Fatalf("references not rewritten: avatar=%q detail=%q", detail.RoleAvatar, detail.RoleDetailDesc)
	}

	// Server report.
	report, err := sync.Validate(ctx, e.client, res.RoleID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status == "blocker" {
		t.Fatalf("validation blockers: %v", report.Blockers)
	}
	if report.TokenBudget == nil || len(report.TokenBudget.Limits) == 0 {
		t.Fatal("report carries no limits")
	}

	// Nothing changed: nothing sent, nothing uploaded.
	again, err := sync.Push(ctx, e.client, f, sync.PushOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Changed) != 0 || len(again.Unchanged) != 4 {
		t.Fatalf("second push: changed=%v unchanged=%v", again.Changed, again.Unchanged)
	}
	for _, a := range again.Assets {
		if a.Uploaded {
			t.Fatalf("asset %s re-uploaded", a.Path)
		}
	}

	// Edit the opening only.
	f.Welcome = "Hello again from the end-to-end test."
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	third, err := sync.Push(ctx, e.client, e.reload(t, f), sync.PushOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Changed) != 1 || third.Changed[0] != card.SectionWelcome {
		t.Fatalf("third push changed=%v", third.Changed)
	}

	// Pull it back and compare.
	out := filepath.Join(t.TempDir(), "pulled")
	pulled, err := sync.Pull(ctx, e.client, res.RoleID, out, sync.PullOptions{Download: true})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	g, err := card.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if g.Manifest.Name != "E2E Card" || card.Body(g.Welcome) != "Hello again from the end-to-end test." {
		t.Fatalf("pulled manifest/welcome: %q %q", g.Manifest.Name, g.Welcome)
	}
	if len(g.Alternates) != 1 || g.Manifest.Prologue[0] != "Begin." {
		t.Fatalf("pulled openings: %+v %v", g.Alternates, g.Manifest.Prologue)
	}
	if g.Lorebook == nil || len(g.Lorebook.Entries) != 2 || g.Lorebook.Entries[0].ID == "" {
		t.Fatalf("pulled lorebook: %+v", g.Lorebook)
	}
	if g.Rules == nil || len(g.Rules.Rules) != 1 {
		t.Fatalf("pulled rules: %+v", g.Rules)
	}
	if len(pulled.Downloaded) != 1 || g.Manifest.Media.Portrait != "assets/portrait.png" {
		t.Fatalf("download: %v portrait=%q", pulled.Downloaded, g.Manifest.Media.Portrait)
	}
	if g.State.RoleID != res.RoleID || g.State.Target != "trial" {
		t.Fatalf("pulled state: %+v", g.State)
	}
}

func TestOwnedPushCreatesAndUpdates(t *testing.T) {
	e := setup(t)
	e.login(t)
	ctx := context.Background()
	f := fixtureFolder(t)

	res, err := sync.Push(ctx, e.client, f, sync.PushOptions{Create: true})
	if err != nil {
		t.Fatalf("create push: %v", err)
	}
	if res.Target != "owned" || !res.Created || res.RoleID == "" {
		t.Fatalf("result: %+v", res)
	}
	if len(res.Changed) != 4 {
		t.Fatalf("changed=%v skipped=%v", res.Changed, res.Skipped)
	}
	f = e.reload(t, f)
	if f.Lorebook.ID == "" || f.Lorebook.Entries[0].ID == "" || f.State.AuthorAssetVersion == 0 {
		t.Fatalf("ids not learned: book=%q entry=%q version=%d", f.Lorebook.ID, f.Lorebook.Entries[0].ID, f.State.AuthorAssetVersion)
	}

	// Remove one entry, edit another, push: update + delete, no duplicates.
	f.Lorebook.Entries = f.Lorebook.Entries[:1]
	f.Lorebook.Entries[0].Content = "A busy harbor."
	f.Rules.Rules[0].Replace = "<strong>$1</strong>"
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	second, err := sync.Push(ctx, e.client, e.reload(t, f), sync.PushOptions{})
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	want := map[string]bool{card.SectionWorldbook: true, card.SectionAuthorAsset: true}
	for _, s := range second.Changed {
		if !want[s] {
			t.Fatalf("unexpected section sent: %v", second.Changed)
		}
		delete(want, s)
	}
	if len(want) != 0 {
		t.Fatalf("sections not sent: %v (changed=%v)", want, second.Changed)
	}
	var entries struct {
		Entries []card.RemoteEntry `json:"entries"`
	}
	if err := e.client.OpenGet(ctx, "/worldbook/entry/list", map[string][]string{"worldbookId": {f.State.LorebookID}}, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries.Entries) != 1 || entries.Entries[0].Content != "A busy harbor." {
		t.Fatalf("remote entries after push: %+v", entries.Entries)
	}
	var asset card.RemoteAsset
	if err := e.client.OpenGet(ctx, "/role/author-asset", map[string][]string{"roleId": {res.RoleID}}, &asset); err != nil {
		t.Fatal(err)
	}
	if len(asset.Rules) != 1 || asset.Rules[0].Replace != "<strong>$1</strong>" {
		t.Fatalf("remote rules: %+v", asset.Rules)
	}

	// Clean up the owned card so the account does not accumulate them.
	if err := e.client.OpenDelete(ctx, "/role/"+res.RoleID, nil); err != nil {
		t.Logf("cleanup delete: %v", err)
	}
}

func (e *env) reload(t *testing.T, f *card.Folder) *card.Folder {
	t.Helper()
	g, err := card.Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
