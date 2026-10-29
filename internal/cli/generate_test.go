package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hearthroom/cli/internal/auth"
)

const genTaskID = "0199aabb-ccdd-7eee-8fff-000011112222"

// genProvider fakes the signed-in generation endpoints and the media library.
type genProvider struct {
	srv        *httptest.Server
	mu         sync.Mutex
	create     string // "200", "202", or an error status
	polls      int
	idemKeys   []string
	genBody    map[string]any
	saveBody   map[string]any
	saveCalls  int
	modelsLang string
}

func newGenProvider(t *testing.T, create string) *genProvider {
	t.Helper()
	p := &genProvider{create: create}
	old := generatePollEvery
	generatePollEvery = time.Millisecond
	t.Cleanup(func() { generatePollEvery = old })
	mux := http.NewServeMux()
	task := func(status string) map[string]any {
		m := map[string]any{"id": genTaskID, "object": "image.generation.task", "model": "image-model-a", "status": status, "credits": 35}
		if status == "completed" {
			m["data"] = []any{map[string]any{"url": p.srv.URL + "/result/1.png"}}
		}
		return m
	}
	mux.HandleFunc("GET /open/v1/generations/models", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.modelsLang = r.URL.Query().Get("language")
		p.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{
			map[string]any{"id": "image-model-a", "name": "Image Model", "type": "image", "lane": "lane-a", "lane_name": "Lane North",
				"pricing":        map[string]any{"unit": "credits_per_image", "credits": map[string]any{"standard": 35}},
				"capabilities":   map[string]any{"sizes": []string{"1024x1024", "1024x1536"}, "max_images": 2},
				"content_policy": map[string]any{"nsfw": "none", "real_person": false}},
			map[string]any{"id": "image-model-b", "name": "Image Model", "type": "image", "lane": "lane-b", "lane_name": "Lane South",
				"pricing":      map[string]any{"unit": "credits_per_image", "credits": map[string]any{"standard": 61}},
				"capabilities": map[string]any{"sizes": []string{"1024x1024"}, "max_images": 4}},
			map[string]any{"id": "some-video", "name": "Video", "type": "video"},
		}})
	})
	mux.HandleFunc("POST /open/v1/generations/images", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.idemKeys = append(p.idemKeys, r.Header.Get("Idempotency-Key"))
		_ = json.NewDecoder(r.Body).Decode(&p.genBody)
		w.Header().Set("X-Harbor-Task-Id", genTaskID)
		switch p.create {
		case "200":
			_ = json.NewEncoder(w).Encode(map[string]any{"created": 1, "data": task("completed")["data"], "harbor": task("completed")})
		case "202":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(task("in_progress"))
		case "403":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"type":"permission_error","code":"permission_error","param":null,"message":"This sign-in was not granted media.generate. Sign in again and allow image and video generation."}}`))
		case "402":
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"error":{"type":"insufficient_quota","code":"insufficient_quota","param":null,"message":"Not enough credits to start this request."}}`))
		}
	})
	mux.HandleFunc("GET /open/v1/generations/{id}", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.polls++
		n := p.polls
		p.mu.Unlock()
		status := "in_progress"
		if n >= 2 {
			status = "completed"
		}
		_ = json.NewEncoder(w).Encode(task(status))
	})
	mux.HandleFunc("POST /open/v1/generations/{id}/save", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.saveCalls++
		_ = json.NewDecoder(r.Body).Decode(&p.saveBody)
		p.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"id": genTaskID, "object": "media.saved", "asset_ids": []string{"asset-1"}})
	})
	mux.HandleFunc("GET /open/v1/image/folder/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"folders": []any{map[string]any{"id": "f-uuid-1", "name": "mira"}}}})
	})
	mux.HandleFunc("GET /open/v1/image/list", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "image-model-a-0199aabb" {
			t.Errorf("library lookup q = %q", r.URL.Query().Get("q"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"imageList": []any{
			map[string]any{"id": "asset-1", "fileName": "mira/image-model-a-0199aabb-1.png", "imageUrl": "https://cdn.test/u/abc/mira/image-model-a-0199aabb-1.png"},
		}}})
	})
	mux.HandleFunc("GET /result/1.png", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("PNGDATA")) })
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	t.Setenv(auth.EnvToken, "tok")
	return p
}

func (p *genProvider) run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return runCLI(t, append(append([]string{"media"}, args...), "--api", p.srv.URL, "--config-dir", t.TempDir())...)
}

// Two models with the same name differ by lane; the lane name is shown and
// asked for in the terminal's language.
func TestMediaModelsListsImageModels(t *testing.T) {
	p := newGenProvider(t, "200")
	t.Setenv("LC_ALL", "zh_TW.UTF-8")
	out, _, err := p.run(t, "models")
	if err != nil {
		t.Fatal(err)
	}
	var rowA, rowB string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "image-model-a "):
			rowA = line
		case strings.HasPrefix(line, "image-model-b "):
			rowB = line
		}
	}
	if !strings.Contains(out, "LANE") || !strings.Contains(rowA, "Lane North") || !strings.Contains(rowA, " 35 ") || !strings.Contains(rowA, "1024x1536") ||
		!strings.Contains(rowB, "Lane South") || !strings.Contains(rowB, " 61 ") || strings.Contains(out, "some-video") {
		t.Fatalf("models output:\n%s", out)
	}
	if p.modelsLang != "zh-Hant" {
		t.Fatalf("language = %q", p.modelsLang)
	}
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	if _, _, err := p.run(t, "models"); err != nil || p.modelsLang != "" {
		t.Fatalf("without a locale: err = %v, language = %q", err, p.modelsLang)
	}
}

func TestMediaGenerateRequiresAllowSpend(t *testing.T) {
	p := newGenProvider(t, "200")
	_, _, err := p.run(t, "generate", "--model", "image-model-a", "--prompt", "a lighthouse")
	if err == nil || !strings.Contains(err.Error(), "--allow-spend") || len(p.idemKeys) != 0 {
		t.Fatalf("err = %v, calls = %d", err, len(p.idemKeys))
	}
}

// 200: the image is ready in the request; --dir saves under that path and
// --out downloads it; the library URL is printed for the card.
func TestMediaGenerateSyncSavesToDirAndDownloads(t *testing.T) {
	p := newGenProvider(t, "200")
	file := filepath.Join(t.TempDir(), "assets", "portrait.png")
	out, _, err := p.run(t, "generate", "--model", "image-model-a", "--prompt", "a lighthouse keeper", "--size", "1024x1536",
		"-n", "1", "--dir", "/mira/", "--out", file, "--allow-spend", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.idemKeys) != 1 || !strings.HasPrefix(p.idemKeys[0], "hearthroom-cli-") {
		t.Fatalf("idempotency keys = %v", p.idemKeys)
	}
	if p.genBody["model"] != "image-model-a" || p.genBody["prompt"] != "a lighthouse keeper" || p.genBody["size"] != "1024x1536" || p.genBody["n"] != float64(1) {
		t.Fatalf("generate body = %v", p.genBody)
	}
	if p.polls != 0 || p.saveBody["directory"] != "mira" {
		t.Fatalf("polls = %d, save body = %v", p.polls, p.saveBody)
	}
	if b, _ := os.ReadFile(file); string(b) != "PNGDATA" {
		t.Fatalf("downloaded %q", b)
	}
	var res struct {
		TaskID string           `json:"taskId"`
		Images []map[string]any `json:"images"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	img := res.Images[0]
	if res.TaskID != genTaskID || img["assetId"] != "asset-1" || img["url"] != "https://cdn.test/u/abc/mira/image-model-a-0199aabb-1.png" || img["file"] != file {
		t.Fatalf("json = %s", out)
	}
}

// 202: the CLI polls the task, then saves into the folder resolved by name.
func TestMediaGenerateAsyncPollsAndSavesToFolder(t *testing.T) {
	p := newGenProvider(t, "202")
	out, _, err := p.run(t, "generate", "--model", "image-model-a", "--prompt", "a storm", "--folder", "mira",
		"--idempotency-key", "order-7", "--allow-spend")
	if err != nil {
		t.Fatal(err)
	}
	if p.idemKeys[0] != "order-7" || p.polls != 2 {
		t.Fatalf("keys = %v, polls = %d", p.idemKeys, p.polls)
	}
	ids, _ := p.saveBody["folderIds"].([]any)
	if len(ids) != 1 || ids[0] != "f-uuid-1" || p.saveBody["directory"] != nil {
		t.Fatalf("save body = %v", p.saveBody)
	}
	if !strings.Contains(out, "https://cdn.test/u/abc/mira/image-model-a-0199aabb-1.png") || !strings.Contains(out, "35 credits") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestMediaGenerateUnknownFolder(t *testing.T) {
	p := newGenProvider(t, "200")
	_, _, err := p.run(t, "generate", "--model", "m", "--prompt", "x", "--folder", "nope", "--allow-spend")
	if err == nil || !strings.Contains(err.Error(), "--dir nope") || len(p.idemKeys) != 0 {
		t.Fatalf("err = %v, generate calls = %d", err, len(p.idemKeys))
	}
}

func TestMediaGenerateWithoutSaveTarget(t *testing.T) {
	p := newGenProvider(t, "200")
	out, _, err := p.run(t, "generate", "--model", "image-model-a", "--prompt-file", "-", "--allow-spend")
	if err == nil || !strings.Contains(err.Error(), "--prompt") {
		t.Fatalf("empty stdin prompt: err = %v", err)
	}
	out, _, err = p.run(t, "generate", "--model", "image-model-a", "--prompt", "x", "--allow-spend")
	if err != nil || p.saveCalls != 0 || !strings.Contains(out, "/result/1.png (kept for 7 days)") {
		t.Fatalf("err = %v, saves = %d, out:\n%s", err, p.saveCalls, out)
	}
}

func TestMediaGenerateMissingScopeSaysLoginAgain(t *testing.T) {
	p := newGenProvider(t, "403")
	_, _, err := p.run(t, "generate", "--model", "m", "--prompt", "x", "--allow-spend")
	if err == nil || !strings.Contains(err.Error(), "media.generate") || !strings.Contains(err.Error(), "hearthroom auth login") {
		t.Fatalf("err = %v", err)
	}
}

func TestMediaGenerateInsufficientPurchasedCredits(t *testing.T) {
	p := newGenProvider(t, "402")
	_, _, err := p.run(t, "generate", "--model", "m", "--prompt", "x", "--allow-spend")
	if err == nil || !strings.Contains(err.Error(), "purchased credits") || !strings.Contains(err.Error(), "gift credits do not count") {
		t.Fatalf("err = %v", err)
	}
}
