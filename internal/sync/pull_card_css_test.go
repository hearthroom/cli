package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
)

func TestPullCSSOnlyAuthorAssetPresence(t *testing.T) {
	text := func(s string) *string { return &s }
	for _, tc := range []struct {
		name, creation string
		css, dialect   *string
		nullFields     bool
	}{
		{"raw CSS and hint", "owned", text("\ufeff\r\n#ai-chat-input{color:red}\r\n"), text("fengyue"), false},
		{"CSS without hint", "trial", text("\r\np{color:blue}\r\n"), nil, false},
		{"explicit empty CSS", "owned", text(""), nil, false},
		{"hint without CSS", "trial", nil, text("fengyue"), false},
		{"explicit empty hint", "owned", nil, text(""), false},
		{"old empty asset defaults", "trial", nil, nil, false},
		{"null fields", "owned", nil, nil, true},
	} {
		for _, force := range []bool{false, true} {
			mode := "fresh"
			if force {
				mode = "force replaces stale files"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				// Same defaults as the provider's emptyAuthorAsset. They do not
				// establish an asset; explicit CSS/hint presence does, even "".
				asset := map[string]any{"status": "none", "version": 0, "rules": []any{}, "mountTrigger": "", "mountLayer": "", "pageMode": "classic", "sharedState": map[string]any{}, "openingPage": "", "openingMode": "greeting"}
				if tc.css != nil {
					asset["cardCss"] = *tc.css
				}
				if tc.dialect != nil {
					asset["cardCssDialect"] = *tc.dialect
				}
				if tc.nullFields {
					asset["cardCss"], asset["cardCssDialect"] = nil, nil
				}
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/open/v1/role/detail":
						json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "synthetic-source", "roleName": "CSS fixture", "language": "en", "creationMethod": tc.creation})
					case "/open/v1/worldbook/bindings":
						json.NewEncoder(w).Encode(map[string]any{"bindings": []any{}})
					case "/open/v1/role/author-asset":
						json.NewEncoder(w).Encode(asset)
					default:
						http.NotFound(w, r)
					}
				}))
				defer srv.Close()
				c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "fixture-token", nil })
				dir := t.TempDir()
				if force {
					old := pullReplacementFixture(t)
					dialect := "tavern"
					old.Rules.CardCSSDialect = &dialect
					if err := old.Save(); err != nil {
						t.Fatal(err)
					}
					dir = old.Dir
				}
				res, err := Pull(context.Background(), c, "synthetic-source", dir, PullOptions{Force: force})
				if err != nil || len(res.Warnings) != 0 {
					t.Fatalf("pull = %+v: %v", res, err)
				}
				f, err := card.Load(dir)
				if err != nil {
					t.Fatal(err)
				}
				if f.State.Target != tc.creation {
					t.Fatalf("existing target semantics changed: %q want %q", f.State.Target, tc.creation)
				}
				if tc.css == nil {
					if _, err := os.Stat(filepath.Join(dir, card.CardCSSFile)); !os.IsNotExist(err) || f.CardCSS != nil {
						t.Fatalf("absent CSS must not retain/create card.css: css=%v stat=%v", f.CardCSS, err)
					}
				} else {
					got, err := os.ReadFile(filepath.Join(dir, card.CardCSSFile))
					if err != nil || string(got) != *tc.css || f.CardCSS == nil || *f.CardCSS != *tc.css {
						t.Fatalf("CSS raw bytes lost: disk=%q loaded=%v want=%q: %v", got, f.CardCSS, *tc.css, err)
					}
				}
				wantAsset := tc.css != nil || tc.dialect != nil
				if !wantAsset {
					if _, err := os.Stat(filepath.Join(dir, card.RulesFile)); !os.IsNotExist(err) || f.Rules != nil {
						t.Fatalf("empty asset defaults must not create/retain rules: %+v %v", f.Rules, err)
					}
				} else if f.Rules == nil || len(f.Rules.Rules) != 0 || (f.Rules.CardCSSDialect == nil) != (tc.dialect == nil) || tc.dialect != nil && *f.Rules.CardCSSDialect != *tc.dialect {
					t.Fatalf("CSS-only metadata lost or stale rules retained: %+v", f.Rules)
				}
				if err := f.Save(); err != nil {
					t.Fatal(err)
				}
				payload, err := f.Build(nil)
				if err != nil {
					t.Fatal(err)
				}
				for field, want := range map[string]*string{"cardCss": tc.css, "cardCssDialect": tc.dialect} {
					got, present := payload.AuthorAsset[field]
					if present != (want != nil) || want != nil && got != *want {
						t.Errorf("next payload %s = %v (present %v), want %v", field, got, present, want)
					}
				}
			})
		}
	}
}
