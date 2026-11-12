package media

import (
	"context"
	"encoding/json"
	"github.com/hearthroom/cli/internal/api"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCardUploadUsesOnlyBoundNamespaceAndReusesCompletionTicket(t *testing.T) {
	intents, puts, completes := 0, 0, 0
	var origin string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/role/card-id/images/uploadIntent":
			intents++
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"uploadId": "stable", "uploadUrl": origin + "/signed-put", "contentType": "image/png"}})
		case "/signed-put":
			puts++
			if r.Header.Get("Authorization") != "" {
				t.Error("provider token leaked to storage")
			}
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != "synthetic image" {
				t.Errorf("bytes: %q", raw)
			}
			w.WriteHeader(http.StatusOK)
		case "/open/v1/role/card-id/images/uploadComplete":
			completes++
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if len(b) != 1 || b["uploadId"] != "stable" {
				t.Errorf("completion includes library path/filename: %v", b)
			}
			if completes == 1 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"imageId": "asset", "imageUrl": "https://cdn.test/card-media/card-id/unique"}})
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	origin = srv.URL
	file := filepath.Join(t.TempDir(), "same.png")
	if err := os.WriteFile(file, []byte("synthetic image"), 0600); err != nil {
		t.Fatal(err)
	}
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "secret", nil })
	got, err := Upload(context.Background(), c, file, "never-library/same.png", "card-id")
	if err != nil || got.ImageURL != "https://cdn.test/card-media/card-id/unique" {
		t.Fatalf("card upload: %+v %v", got, err)
	}
	if intents != 1 || puts != 1 || completes != 2 {
		t.Fatalf("intent/put/complete=%d/%d/%d", intents, puts, completes)
	}
}
