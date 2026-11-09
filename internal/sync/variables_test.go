package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/importer"
)

// variablesProvider stores the state section the way the provider does
// (normalized once) and serves it back in the role detail.
type variablesProvider struct {
	t        *testing.T
	stored   map[string]any // author view, nil when the card has none
	requests []string
	trial    map[string]any
	reject   string
}

func (vp *variablesProvider) store(body map[string]any) {
	raw, _ := json.Marshal(body["data"])
	data, err := card.ParseJSON(raw)
	if err != nil {
		vp.t.Fatal(err)
	}
	conv, _ := body["convention"].(string)
	var paths map[string]string
	rulesText := ""
	if r, ok := body["rules"].(map[string]any); ok {
		rulesText, _ = r["text"].(string)
		if p, ok := r["paths"].(map[string]any); ok {
			paths = map[string]string{}
			for k, v := range p {
				paths[k] = v.(string)
			}
		}
	}
	n, reason := card.NormalizeVariables(conv, data, paths)
	if reason != "" {
		vp.t.Fatalf("provider would reject: %s", reason)
	}
	view := map[string]any{"version": 2, "data": json.RawMessage(n.Data.Compact()), "rules": map[string]any{"text": rulesText, "paths": n.Paths}, "open": nonNilStrings(n.Open), "required": nonNilStrings(n.Required)}
	if p, ok := body["panel"]; ok {
		view["panel"] = p
	}
	vp.stored = view
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (vp *variablesProvider) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vp.requests = append(vp.requests, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/open/v1/role/detail":
			d := map[string]any{"characterRoleId": "owned", "roleName": "Vars", "language": "en", "roleWelcome": "Hello.", "roleDetailDesc": "Definition."}
			if vp.stored != nil {
				d["state"] = vp.stored
			}
			json.NewEncoder(w).Encode(d)
		case r.URL.Path == "/open/v1/worldbook/bindings":
			json.NewEncoder(w).Encode(map[string]any{"bindings": []any{}})
		case r.URL.Path == "/open/v1/role/author-asset":
			http.NotFound(w, r)
		case strings.HasPrefix(r.URL.Path, "/open/v1/trial-cards/"):
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			vp.trial = body
			if vp.reject != "" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{"error": "trial_unsupported", "detail": map[string]any{"section": "state", "reason": vp.reject}})
				return
			}
			if st, ok := body["state"].(map[string]any); ok {
				vp.store(st)
			} else {
				vp.stored = nil
			}
			json.NewEncoder(w).Encode(map[string]any{"roleId": "trial-role", "created": true, "sections": map[string]string{}, "changed": []string{"state"}})
		case r.URL.Path == "/open/v1/role/owned/state" && r.Method == http.MethodPut:
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if vp.reject != "" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{"error": "validate_reject", "detail": map[string]any{"reason": vp.reject}})
				return
			}
			vp.store(body)
			json.NewEncoder(w).Encode(map[string]any{"state": vp.stored})
		case r.URL.Path == "/open/v1/role/owned/state" && r.Method == http.MethodDelete:
			vp.stored = nil
			w.Write([]byte(`{"state":null}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
}

func (vp *variablesProvider) count(req string) int {
	n := 0
	for _, r := range vp.requests {
		if r == req {
			n++
		}
	}
	return n
}

const mvuSkeleton = `{
  "convention": "mvu",
  "data": {
    "好感度": [50, "+1 per kind act"],
    "背包": {"$meta": {"extensible": true}, "錢": [10, "spent on purchases"]},
    "裝備": [["短刀", "鐵護腕"], ""]
  }
}
`

func writeVariables(t *testing.T, dir, data, rules, panel string) *card.Folder {
	t.Helper()
	for name, text := range map[string]string{card.VariablesFile: data, card.VariableRulesFile: rules, card.PanelFile: panel} {
		path := filepath.Join(dir, name)
		if text == "-" {
			os.Remove(path)
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// The trial document carries the variables as the state section, key order
// intact; a folder without variables.json leaves the section out, which the
// provider reads as "remove".
func TestTrialPushSendsVariables(t *testing.T) {
	vp := &variablesProvider{t: t}
	srv := vp.server()
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	dir := t.TempDir()
	if _, err := card.Init(dir, "Vars", "en"); err != nil {
		t.Fatal(err)
	}
	f := writeVariables(t, dir, mvuSkeleton, "Values follow the story.\n", "<b>{{好感度}}</b>\n")
	res, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	st, ok := vp.trial["state"].(map[string]any)
	if !ok {
		t.Fatalf("trial body has no state: %v", vp.trial)
	}
	raw, _ := json.Marshal(st["data"])
	if !strings.HasPrefix(string(raw), `{"好感度":[50,"+1 per kind act"],"背包"`) {
		t.Fatalf("data order lost: %s", raw)
	}
	if st["convention"] != "mvu" || st["rules"].(map[string]any)["text"] != "Values follow the story." || st["panel"].(map[string]any)["template"] != "<b>{{好感度}}</b>" {
		t.Fatalf("state = %v", st)
	}
	if strings.Join(res.Changed, ",") != "state" {
		t.Fatalf("changed = %v", res.Changed)
	}

	f = writeVariables(t, dir, "-", "-", "-")
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := vp.trial["state"]; ok {
		t.Fatalf("state sent without variables.json: %v", vp.trial["state"])
	}
}

// Owned cards: PUT when the variables change, nothing when they do not, nothing
// when the file is absent (the card's variables are left alone, as with
// lorebook.json), and DELETE once when the file is emptied.
func TestOwnedPushWritesAndRemovesVariables(t *testing.T) {
	vp := &variablesProvider{t: t}
	srv := vp.server()
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	dir := t.TempDir()
	if _, err := card.Init(dir, "Vars", "en"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, card.WelcomeFile), []byte("Hello."), 0o644)
	f := writeVariables(t, dir, mvuSkeleton, "-", "-")
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true, To: "owned"}); err != nil {
		t.Fatal(err)
	}
	f, _ = card.Load(dir)
	res, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if vp.count("PUT /open/v1/role/owned/state") != 1 || !contains(res.Unchanged, "state") {
		t.Fatalf("requests = %v, unchanged = %v", vp.requests, res.Unchanged)
	}

	f = writeVariables(t, dir, "-", "-", "-")
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	if vp.count("DELETE /open/v1/role/owned/state") != 0 || vp.stored == nil {
		t.Fatalf("absent variables.json removed the card's variables: %v", vp.requests)
	}

	f = writeVariables(t, dir, "\n", "-", "-")
	res, err = Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if vp.count("DELETE /open/v1/role/owned/state") != 1 || vp.stored != nil || !contains(res.Changed, "state") {
		t.Fatalf("empty variables.json: requests %v changed %v", vp.requests, res.Changed)
	}
	f, _ = card.Load(dir)
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	if vp.count("DELETE /open/v1/role/owned/state") != 1 {
		t.Fatalf("removal sent twice: %v", vp.requests)
	}
}

// push → pull → push stores the same definition and writes the same files.
func TestPullRebuildsVariablesStably(t *testing.T) {
	vp := &variablesProvider{t: t}
	srv := vp.server()
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	src := t.TempDir()
	if _, err := card.Init(src, "Vars", "en"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, card.WelcomeFile), []byte("Hello."), 0o644)
	f := writeVariables(t, src, mvuSkeleton, "Values follow the story.\n", "<b>{{好感度}}</b>\n")
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true, To: "owned"}); err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(vp.stored)

	dir := t.TempDir()
	res, err := Pull(context.Background(), c, "owned", dir, PullOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{card.VariablesFile, card.VariableRulesFile, card.PanelFile} {
		if !contains(res.Files, name) {
			t.Fatalf("pull files = %v, want %s", res.Files, name)
		}
	}
	pulled, _ := os.ReadFile(filepath.Join(dir, card.VariablesFile))
	if !strings.Contains(string(pulled), `"好感度": [50, "+1 per kind act"]`) || !strings.Contains(string(pulled), `"extensible": true`) {
		t.Fatalf("pulled variables.json:\n%s", pulled)
	}
	f, _ = card.Load(dir)
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	second, _ := json.Marshal(vp.stored)
	if string(first) != string(second) {
		t.Fatalf("definition changed after pull and push:\n%s\n%s", first, second)
	}
	if _, err := Pull(context.Background(), c, "owned", dir, PullOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(dir, card.VariablesFile))
	if string(again) != string(pulled) {
		t.Fatalf("second pull wrote a different file:\n%s\n---\n%s", pulled, again)
	}

	// The card's variables are gone: pull removes the three files.
	vp.stored = nil
	if _, err := Pull(context.Background(), c, "owned", dir, PullOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{card.VariablesFile, card.VariableRulesFile, card.PanelFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s kept after the card's variables were removed: %v", name, err)
		}
	}
}

// Limits are checked before anything is written; a provider rejection says
// which file to fix.
func TestVariablesPreflightAndRejection(t *testing.T) {
	vp := &variablesProvider{t: t}
	srv := vp.server()
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	dir := t.TempDir()
	if _, err := card.Init(dir, "Vars", "en"); err != nil {
		t.Fatal(err)
	}
	f := writeVariables(t, dir, `{"convention":"plain","data":{"a":1}}`, "-", strings.Repeat("x", card.VariablesPanelMax+1))
	_, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err == nil || !strings.Contains(err.Error(), card.PanelFile) || len(vp.requests) != 0 {
		t.Fatalf("err = %v, requests = %v", err, vp.requests)
	}

	f = writeVariables(t, dir, `{"convention":"plain","data":{"a":1}}`, "-", "-")
	vp.reject = "rule_path"
	_, err = Push(context.Background(), c, f, PushOptions{SkipMedia: true})
	if err == nil || !strings.Contains(err.Error(), "a rule does not match any value") {
		t.Fatalf("trial rejection = %v", err)
	}
	_, err = Push(context.Background(), c, f, PushOptions{SkipMedia: true, To: "owned"})
	if err == nil || !strings.Contains(err.Error(), "refused the card variables") {
		t.Fatalf("owned rejection = %v", err)
	}
}

// An AI Fengyue import (card format fengyue, variable panel included) passes
// the push preflight as written.
func TestFengyueImportPushes(t *testing.T) {
	raw, ok := importer.DecodeFengyue(fengyueSample(t))
	if !ok {
		t.Fatal("sample not detected")
	}
	raw["ai_v_json"] = `{"好感度": 10, "物品": {}}`
	raw["ai_variable_template"] = "<b>{{好感度}}</b>"
	dir := filepath.Join(t.TempDir(), "card")
	if _, err := importer.WriteFolder(importer.FromFengyue(raw, "zh-Hans"), dir, false); err != nil {
		t.Fatal(err)
	}
	vp := &variablesProvider{t: t}
	srv := vp.server()
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), c, f, PushOptions{SkipMedia: true}); err != nil {
		t.Fatal(err)
	}
	st, _ := vp.trial["state"].(map[string]any)
	if st["convention"] != "plain" || st["panel"] == nil {
		t.Fatalf("state = %v", st)
	}
	if aa, _ := vp.trial["authorAsset"].(map[string]any); aa["cardFormat"] != "fengyue" {
		t.Fatalf("author asset = %v", vp.trial["authorAsset"])
	}
}

func fengyueSample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../importer/testdata/fengyue-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Input json.RawMessage `json:"input"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	return file.Cases[0].Input
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
