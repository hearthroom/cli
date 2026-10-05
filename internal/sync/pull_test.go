package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"github.com/hearthroom/cli/internal/media"
)

// A pull rebuilds card.json and the sync state, but the next push must keep
// writing into the card's own media folder instead of treating it as foreign.
func TestPullKeepsTheMediaFolder(t *testing.T) {
	const prefix = "https://cdn.test/u/abc"
	avatar := prefix + "/" + url.PathEscape("天道非要我成仙") + "/art/p.webp"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/role/detail":
			json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "r1", "roleName": "天道非要我成仙", "roleAvatar": avatar, "roleBackground": avatar})
		case "/open/v1/worldbook/bindings":
			json.NewEncoder(w).Encode(map[string]any{"bindings": []any{}})
		case "/open/v1/image/list":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"libraryPrefix": prefix, "capabilities": map[string]any{"relativePaths": true}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })

	// A fresh folder learns the folder the card's images already live in.
	dir := t.TempDir()
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{}); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _, fresh := media.Folder(f); got != "天道非要我成仙" || fresh {
		t.Fatalf("fresh pull: folder %q fresh=%v", got, fresh)
	}

	// Pulling over a folder keeps the folder the author chose.
	f.Manifest.Media.Folder = "series/tiandao"
	f.State.AssetFolder = "series/tiandao"
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveState(); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(context.Background(), c, "r1", dir, PullOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	f, err = card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Manifest.Media.Folder != "series/tiandao" || f.State.AssetFolder != "series/tiandao" {
		t.Fatalf("pull over folder: manifest %q state %q", f.Manifest.Media.Folder, f.State.AssetFolder)
	}
}
