package sync

import (
	"context"
	"encoding/json"
	"github.com/hearthroom/cli/internal/api"
	"github.com/hearthroom/cli/internal/card"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPushSeparatesCardPicturesFromInlineLibraryAssets(t *testing.T) {
	var origin string
	var named []string
	var fields map[string]any
	intents := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/open/v1/role/detail":
			json.NewEncoder(w).Encode(map[string]any{"roleName": "Scope", "language": "en", "roleVisibility": "private"})
		case r.URL.Path == "/open/v1/image/list":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"imageList": []any{}, "capabilities": map[string]any{"relativePaths": true}, "libraryPrefix": "https://cdn.test/u/owner"}})
		case r.URL.Path == "/open/v1/image/upload":
			r.ParseMultipartForm(1 << 20)
			path := r.FormValue("relativePath")
			named = append(named, path)
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"imageId": "library", "imageUrl": "https://cdn.test/u/owner/" + path, "fileName": path}})
		case r.URL.Path == "/open/v1/role/existing/images/uploadIntent":
			intents++
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"uploadId": "cover-ticket", "uploadUrl": origin + "/put-cover", "contentType": "image/png"}})
		case r.URL.Path == "/put-cover":
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/open/v1/role/existing/images/uploadComplete":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"imageId": "cover", "imageUrl": "https://cdn.test/card-media/existing/unique"}})
		case strings.HasSuffix(r.URL.Path, "/document"):
			var body struct{ Fields map[string]any }
			json.NewDecoder(r.Body).Decode(&body)
			fields = body.Fields
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	origin = srv.URL
	f, err := card.Init(t.TempDir(), "Scope", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(f.Dir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cover.png", "inline.png"} {
		if err = os.WriteFile(filepath.Join(f.Dir, "assets", name), []byte("synthetic "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f.Manifest.Media.Background = "assets/cover.png"
	f.Definition = "A sign: assets/inline.png"
	f.Welcome = "Opening"
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "token", nil })
	if _, err = Push(t.Context(), c, f, PushOptions{To: "existing"}); err != nil {
		t.Fatal(err)
	}
	if len(named) != 1 || !strings.HasSuffix(named[0], "/inline.png") {
		t.Fatalf("card picture consumed library storage: %v", named)
	}
	if intents != 1 || fields["roleBackground"] != "https://cdn.test/card-media/existing/unique" {
		t.Fatalf("card image: %d %v", intents, fields)
	}
	if !strings.Contains(fields["roleDetailDesc"].(string), "/u/owner/") {
		t.Fatalf("inline asset lost library path: %v", fields)
	}
}

func TestNewOwnedCardPicturePersistsProvisionIntentBeforeUploadAndResumesLostReply(t *testing.T) {
	var origin, firstOperation string
	calls, uploads, completed := 0, 0, 0
	var folder *card.Folder
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/me":
			json.NewEncoder(w).Encode(map[string]any{"accountNumId": 17})
		case "/open/v1/role/provision":
			calls++
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			state, _ := os.ReadFile(filepath.Join(folder.Dir, ".hearthroom", "state.json"))
			operation, _ := req["operationId"].(string)
			if operation == "" || !strings.Contains(string(state), operation) {
				t.Errorf("operation was not saved before request: %s / %v", state, req)
			}
			if firstOperation == "" {
				firstOperation = operation
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			if firstOperation != operation {
				t.Error("lost provision reply changed operation identity")
			}
			json.NewEncoder(w).Encode(map[string]any{"roleId": "created-card", "roleVisibility": "private", "provisioning": true})
		case "/open/v1/role/created-card/images/uploadIntent":
			uploads++
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"uploadId": "t", "uploadUrl": origin + "/put", "contentType": "image/png"}})
		case "/put":
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		case "/open/v1/role/created-card/images/uploadComplete":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"imageId": "i", "imageUrl": "https://cdn.test/card-media/created-card/unique"}})
		case "/open/v1/role/created-card/provisionComplete":
			completed++
			json.NewEncoder(w).Encode(map[string]any{"roleId": "created-card", "roleVisibility": "unlisted", "provisioning": false})
		case "/open/v1/role/created-card/document", "/open/v1/role/created-card/welcome":
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected personal-library or creation route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	origin = srv.URL
	f, err := card.Init(t.TempDir(), "New picture", "en")
	if err != nil {
		t.Fatal(err)
	}
	folder = f
	os.MkdirAll(filepath.Join(f.Dir, "assets"), 0700)
	os.WriteFile(filepath.Join(f.Dir, "assets", "same.png"), []byte("synthetic"), 0600)
	f.Manifest.Media.Background = "assets/same.png"
	f.Welcome = "Opening"
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if _, err = Push(t.Context(), c, f, PushOptions{Create: true}); err == nil {
		t.Fatal("lost first reply should retain pending intent")
	}
	if uploads != 0 {
		t.Fatal("image uploaded before its card identity was confirmed")
	}
	if _, err = Push(t.Context(), c, f, PushOptions{Create: true}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || uploads != 1 || completed != 1 {
		t.Fatalf("create/upload/complete %d/%d/%d", calls, uploads, completed)
	}
}

func TestTrialPicturesEnsureTargetBeforeUploadingAndPreserveTrialIdentity(t *testing.T) {
	var origin, key string
	ensured, uploads, puts := 0, 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/open/v1/me":
			json.NewEncoder(w).Encode(map[string]any{"accountNumId": 17})
		case strings.HasPrefix(r.URL.Path, "/open/v1/trial-cards/") && strings.HasSuffix(r.URL.Path, "/ensure"):
			ensured++
			key = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/open/v1/trial-cards/"), "/ensure")
			json.NewEncoder(w).Encode(map[string]any{"roleId": "trial-image-card", "clientKey": key, "created": true})
		case r.URL.Path == "/open/v1/role/trial-image-card/images/uploadIntent":
			uploads++
			if ensured != 1 {
				t.Error("uploaded before ensure")
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"uploadId": "t", "uploadUrl": origin + "/put", "contentType": "image/png"}})
		case r.URL.Path == "/put":
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/open/v1/role/trial-image-card/images/uploadComplete":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"imageId": "i", "imageUrl": "https://cdn.test/card-media/trial-image-card/unique"}})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/open/v1/trial-cards/"):
			puts++
			if !strings.HasSuffix(r.URL.Path, "/"+key) {
				t.Error("trial key changed after ensure")
			}
			json.NewEncoder(w).Encode(map[string]any{"roleId": "trial-image-card", "created": false, "sections": map[string]string{}, "changed": []string{"card"}})
		case r.URL.Path == "/open/v1/role/trial-image-card/document":
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	origin = srv.URL
	f, err := card.Init(t.TempDir(), "Trial picture", "en")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(f.Dir, "assets"), 0700)
	os.WriteFile(filepath.Join(f.Dir, "assets", "same.png"), []byte("synthetic"), 0600)
	f.Manifest.Media.Background = "assets/same.png"
	f.Welcome = "Opening"
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	result, err := Push(t.Context(), c, f, PushOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ensured != 1 || uploads != 1 || puts != 1 || result.RoleID != "trial-image-card" || !result.Created || f.State.TrialKey != key {
		t.Fatalf("trial target: %+v (%d/%d/%d) key=%q", result, ensured, uploads, puts, key)
	}
}

func TestPullDownloadedCardImagePushKeepsItsScopedURL(t *testing.T) {
	var origin string
	uploadCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/role/detail":
			json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "existing", "roleName": "Scoped pull", "language": "en", "roleAvatar": origin + "/card-media/existing/unique", "roleBackground": origin + "/card-media/existing/unique", "roleWelcome": "Opening"})
		case "/open/v1/worldbook/bindings":
			json.NewEncoder(w).Encode(map[string]any{"bindings": []any{}})
		case "/open/v1/role/author-asset":
			http.NotFound(w, r)
		case "/open/v1/image/list":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"imageList": []any{}, "libraryPrefix": origin + "/u/owner", "capabilities": map[string]any{"relativePaths": true, "cardImages": true}}})
		case "/card-media/existing/unique":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("synthetic image"))
		case "/open/v1/role/existing/document", "/open/v1/role/existing/welcome":
			w.WriteHeader(http.StatusOK)
		default:
			uploadCalls++
			t.Errorf("unexpected upload after unchanged pull: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	origin = srv.URL
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	dir := t.TempDir()
	if _, err := Pull(t.Context(), c, "existing", dir, PullOptions{Download: true}); err != nil {
		t.Fatal(err)
	}
	f, err := card.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.State.Assets) != 0 || f.State.CardImages["roleBackground"].URL != origin+"/card-media/existing/unique" {
		t.Fatalf("pull mixed own image into the library state: %+v", f.State)
	}
	if !strings.HasSuffix(f.Manifest.Media.Portrait, ".png") {
		t.Fatalf("opaque card URL produced an unuploadable filename: %s", f.Manifest.Media.Portrait)
	}
	if _, err = Push(t.Context(), c, f, PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if uploadCalls != 0 {
		t.Fatalf("unchanged image reuploaded: %d", uploadCalls)
	}
}

func TestPushResumesLostCardProvisionCompletionWithoutRepeatingIt(t *testing.T) {
	finishes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/v1/role/detail":
			json.NewEncoder(w).Encode(map[string]any{"characterRoleId": "existing", "roleName": "Completed remotely", "roleWelcome": "Opening", "language": "en", "roleVisibility": "unlisted", "provisioning": false})
		case "/open/v1/me":
			json.NewEncoder(w).Encode(map[string]any{"accountNumId": 7})
		case "/open/v1/role/existing/provisionComplete":
			finishes++
			w.WriteHeader(http.StatusConflict)
		default:
			json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()
	f, err := card.Init(t.TempDir(), "Local edit", "en")
	if err != nil {
		t.Fatal(err)
	}
	f.Welcome = "Opening"
	f.State.API = srv.URL
	f.State.RoleID = "existing"
	f.State.Target = "owned"
	f.State.OwnedPushPending = true
	f.State.CardImageProvision = &card.CardImageProvision{Target: "owned", API: srv.URL, Actor: 7, OperationID: "stable", Name: "Completed remotely", Language: "en"}
	c := api.New(srv.URL, srv.URL, "test", func(context.Context) (string, error) { return "tok", nil })
	if _, err = Push(t.Context(), c, f, PushOptions{Create: true}); err != nil {
		t.Fatal(err)
	}
	if finishes != 0 || f.State.CardImageProvision != nil {
		t.Fatalf("completed card was finalized again: %d %+v", finishes, f.State)
	}
}
