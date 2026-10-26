package media

import (
	"context"
	"encoding/json"
	"github.com/hearthroom/cli/internal/api"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hearthroom/cli/internal/card"
)

func TestDryRunUsesServerNormalizedLibraryURL(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Paths", "en")
	f.Manifest.Media.Folder = "safe"
	os.MkdirAll(filepath.Join(f.Dir, "assets"), 0755)
	rel := "assets/cafe\u0301.svg"
	os.WriteFile(filepath.Join(f.Dir, rel), []byte("svg"), 0644)
	f.Manifest.Media.Portrait = rel
	urls, _, err := Sync(context.Background(), nil, f, Capabilities{RelativePaths: true, LibraryPrefix: "https://cdn.example/u/owner"}, SyncOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if urls[rel] != "https://cdn.example/u/owner/safe/caf%C3%A9.svg" {
		t.Fatalf("planned URL=%q", urls[rel])
	}
}

func TestDryRunDoesNotMutateExistingAssetMap(t *testing.T) {
	f, _ := card.Init(t.TempDir(), "Moves", "en")
	f.State.AssetFolder = "old"
	f.State.Assets = map[string]card.Asset{"assets/icon.svg": {ID: "1", URL: "https://cdn/u/owner/old/icon.svg"}}
	before, _ := json.Marshal(f.State)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"total":1,"imageList":[{"id":"1","imageUrl":"https://cdn/u/owner/new/icon.svg"}]}}`))
	}))
	defer srv.Close()
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	_, _, _ = Sync(context.Background(), c, f, Capabilities{RelativePaths: true, LibraryPrefix: "https://cdn/u/owner"}, SyncOptions{DryRun: true})
	after, _ := json.Marshal(f.State)
	if string(before) != string(after) {
		t.Fatalf("dry-run changed caller state: %s -> %s", before, after)
	}
}
