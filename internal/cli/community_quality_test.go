package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommunityCardQualitySignal(t *testing.T) {
	cfg := authTestEnv(t)
	const length = `{"id":"100001","name":"Card","qualitySignal":{"lengthCharacters":0,"deepPlay":null}}`
	const deep = `{"id":"100002","name":"Second","qualitySignal":{"lengthCharacters":12000,"deepPlay":{"rate":82}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cards":
			_, _ = w.Write([]byte(`{"items":[` + length + `,` + deep + `,{"id":"100003","name":"Older"}]}`))
		case "/v1/cards/100002":
			_, _ = w.Write([]byte(deep))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	run := func(args ...string) string {
		t.Helper()
		stdout, _, err := runCLI(t, append(args, "--site", srv.URL, "--config-dir", cfg)...)
		if err != nil {
			t.Fatal(err)
		}
		return stdout
	}

	board := run("search")
	for _, want := range []string{"under 1,000 chars", "82% retention rate", "Older"} {
		if !strings.Contains(board, want) {
			t.Errorf("board missing %q: %s", want, board)
		}
	}
	if got := run("card", "view", "100002"); !strings.Contains(got, "82% retention rate") {
		t.Errorf("details missing retention rate: %s", got)
	}
	var decoded map[string]any
	if raw := run("card", "view", "100002", "--json"); json.Unmarshal([]byte(raw), &decoded) != nil || decoded["qualitySignal"] == nil {
		t.Fatalf("JSON lost server projection: %s", raw)
	}
	signal := decoded["qualitySignal"].(map[string]any)
	deepPlay := signal["deepPlay"].(map[string]any)
	if len(deepPlay) != 1 || deepPlay["rate"] != float64(82) {
		t.Fatalf("unexpected public retention projection: %#v", deepPlay)
	}
}
