package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/api"
)

func TestLibraryUploadPutsBytesInStorageThenCompletesNamedPath(t *testing.T) {
	local := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(local, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var requests []string
	var mu sync.Mutex
	record := func(r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, r.Method+" "+r.URL.Path)
	}
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		data, err := io.ReadAll(r.Body)
		if err != nil || string(data) != "fixture" || r.Header.Get("Content-Type") != "image/png" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected storage request: bytes=%q type=%q auth=%t err=%v", data, r.Header.Get("Content-Type"), r.Header.Get("Authorization") != "", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer storage.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record(r)
		switch r.URL.Path {
		case "/open/v1/image/uploadIntent":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"uploadId": "ticket", "uploadUrl": storage.URL + "/staging", "contentType": "image/png"}})
		case "/open/v1/image/uploadComplete":
			var req struct {
				UploadID     string `json:"uploadId"`
				FileName     string `json:"fileName"`
				RelativePath string `json:"relativePath"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UploadID != "ticket" || req.FileName != "cover.png" || req.RelativePath != "scenes/cover.png" {
				t.Errorf("unexpected completion request: %+v, %v", req, err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"imageUrl": storage.URL + "/public", "replaced": true}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	c := api.New(provider.URL, provider.URL, "test", func(context.Context) (string, error) { return "token", nil })
	res, err := Upload(t.Context(), c, local, "scenes/cover.png", "")
	mu.Lock()
	defer mu.Unlock()
	if err != nil || res.ImageURL != storage.URL+"/public" || !res.Replaced {
		t.Fatalf("named library upload = %+v, %v, requests %v", res, err, requests)
	}
	want := []string{"POST /open/v1/image/uploadIntent", "PUT /staging", "POST /open/v1/image/uploadComplete"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestLibraryUploadRestoresStagingForSameTicketAfterInterruptedCompletion(t *testing.T) {
	oldDelay := retryBase
	retryBase = time.Millisecond
	t.Cleanup(func() { retryBase = oldDelay })
	var puts, completions atomic.Int32
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		puts.Add(1)
		if body, err := io.ReadAll(r.Body); err != nil || string(body) != "x" {
			t.Errorf("restaged body = %q, %v", body, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer storage.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/image/uploadIntent":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"uploadId": "first-ticket", "uploadUrl": storage.URL, "contentType": "image/png"}})
		case "/open/v1/image/uploadComplete":
			var in struct{ UploadID string }
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.UploadID != "first-ticket" {
				t.Errorf("completion did not reuse original ticket: %+v, %v", in, err)
			}
			switch completions.Add(1) {
			case 1:
				http.Error(w, "temporarily_unavailable", http.StatusInternalServerError)
			case 2:
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"error":"upload_missing"}`)
			default:
				if puts.Load() != 2 {
					t.Errorf("completion ran without restoring staging")
				}
				_, _ = io.WriteString(w, `{"code":0,"data":{"imageUrl":"https://fixture.test/cover"}}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	c := api.New(provider.URL, provider.URL, "test", func(context.Context) (string, error) { return "token", nil })
	res, err := Upload(t.Context(), c, writeFile(t, "cover.png"), "cover.png", "")
	if err != nil || res.ImageURL == "" || puts.Load() != 2 || completions.Load() != 3 {
		t.Fatalf("restaged upload = %+v, %v (PUTs %d, completions %d)", res, err, puts.Load(), completions.Load())
	}
}
