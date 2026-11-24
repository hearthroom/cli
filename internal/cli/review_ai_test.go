package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
)

type reviewAIRequest struct {
	method, path string
	query        url.Values
	body         map[string]any
}

// reviewAIServer records requests and answers each with status and reply.
func reviewAIServer(t *testing.T, status int, reply string) (*httptest.Server, *[]reviewAIRequest) {
	t.Helper()
	var got []reviewAIRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer on %s %s", r.Method, r.URL)
		}
		req := reviewAIRequest{method: r.Method, path: r.URL.EscapedPath(), query: r.URL.Query()}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &req.body); err != nil {
				t.Errorf("body is not a JSON object: %s", raw)
			}
		}
		got = append(got, req)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func reviewAIArgs(srv *httptest.Server, cfg string, args ...string) []string {
	return append(args, "--api", srv.URL, "--site", srv.URL, "--config-dir", cfg)
}

func TestReviewQueueQuery(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, got := reviewAIServer(t, 200, `{"count":3}`)
	out, _, err := runCLI(t, reviewAIArgs(srv, cfg, "review", "queue", "--scope", "recent", "--unshadowed", "--criteria", "v2", "--count", "--limit", "20")...)
	if err != nil {
		t.Fatal(err)
	}
	var printed map[string]any
	if err := json.Unmarshal([]byte(out), &printed); err != nil || printed["count"] != float64(3) {
		t.Fatalf("output %q: %v", out, err)
	}
	req := (*got)[0]
	want := map[string]string{"scope": "recent", "unshadowed": "1", "criteria": "v2", "count": "1", "limit": "20"}
	if req.method != "GET" || req.path != "/v1/review-ai/queue" || len(req.query) != len(want) {
		t.Fatalf("request %+v", req)
	}
	for k, v := range want {
		if q := req.query[k]; len(q) != 1 || q[0] != v {
			t.Fatalf("query %s = %v, want %s", k, q, v)
		}
	}

	*got = nil
	if _, _, err := runCLI(t, reviewAIArgs(srv, cfg, "review", "queue")...); err != nil {
		t.Fatal(err)
	}
	if q := (*got)[0].query; len(q) != 2 || q.Get("scope") != "pending" || q.Get("limit") != "50" {
		t.Fatalf("default query %v", q)
	}
}

func TestReviewSuggestFromFlags(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, got := reviewAIServer(t, 200, `{"id":"s-1","verdict":"reject","phase":"shadow","criteriaVersion":"v1"}`)
	out, _, err := runCLI(t, reviewAIArgs(srv, cfg, "review", "suggest", "sub/1",
		"--criteria", "v1", "--model", "m-1", "--verdict", "reject",
		"--reason", "r1", "--reason", "r2", "--flag", "f1", "--confidence", "0.8",
		"--note", "short note", "--input-tokens", "1200", "--latency-ms", "900")...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"verdict": "reject"`) {
		t.Fatalf("output %s", out)
	}
	req := (*got)[0]
	if req.method != "PUT" || req.path != "/v1/review-ai/submissions/sub%2F1/suggestion" {
		t.Fatalf("request %s %s", req.method, req.path)
	}
	want := `{"confidence":0.8,"criteriaVersion":"v1","flags":["f1"],"inputTokens":1200,"latencyMs":900,"model":"m-1","note":"short note","reasons":["r1","r2"],"verdict":"reject"}`
	if b, _ := json.Marshal(req.body); string(b) != want {
		t.Fatalf("body %s\nwant %s", b, want)
	}

	*got = nil
	if _, _, err := runCLI(t, reviewAIArgs(srv, cfg, "review", "suggest", "sub-2", "--criteria", "v1", "--model", "m-1", "--verdict", "pass")...); err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal((*got)[0].body); string(b) != `{"criteriaVersion":"v1","flags":[],"model":"m-1","reasons":[],"verdict":"pass"}` {
		t.Fatalf("minimal body %s", b)
	}
}

func TestReviewSuggestFromStdin(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, got := reviewAIServer(t, 200, `{"id":"s-1","verdict":"handoff","phase":"shadow","criteriaVersion":"v3"}`)
	app, out, _ := testApp()
	app.In = strings.NewReader(`{"verdict":"handoff","handoffCause":"unsure","reasons":[],"flags":["f1"],"confidence":0.4,"note":"n","model":"from-file","outputTokens":12345678901}`)
	root := app.rootCommand()
	root.SetArgs(reviewAIArgs(srv, cfg, "review", "suggest", "sub-1", "--file", "-", "--criteria", "v3", "--model", "m-2"))
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"criteriaVersion": "v3"`) {
		t.Fatalf("output %s", out)
	}
	want := `{"confidence":0.4,"criteriaVersion":"v3","flags":["f1"],"handoffCause":"unsure","model":"m-2","note":"n","outputTokens":12345678901,"reasons":[],"verdict":"handoff"}`
	if b, _ := json.Marshal((*got)[0].body); string(b) != want {
		t.Fatalf("body %s\nwant %s", b, want)
	}
}

func TestReviewSuggestRejectsBadInput(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, got := reviewAIServer(t, 200, `{}`)
	for name, args := range map[string][]string{
		"verdict":  {"--criteria", "v1", "--model", "m", "--verdict", "approve"},
		"criteria": {"--criteria", "1", "--model", "m", "--verdict", "pass"},
		"missing":  {"--model", "m", "--verdict", "pass"},
		"file":     {"--file", "-"},
	} {
		app, _, _ := testApp()
		app.In = strings.NewReader(`[1]`)
		root := app.rootCommand()
		root.SetArgs(reviewAIArgs(srv, cfg, append([]string{"review", "suggest", "sub-1"}, args...)...))
		if err := root.Execute(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("sent %d requests for invalid input", len(*got))
	}
}

func TestReviewTagShowReport(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, got := reviewAIServer(t, 200, `{"ok":true}`)
	for _, args := range [][]string{
		{"review", "tag", "sub-1", "--criteria", "v1", "--model", "m", "--flag", "f1", "--flag", "f2"},
		{"review", "show", "sub-1"},
		{"review", "report", "--from", "1000", "--to", "2000", "--criteria", "v1"},
	} {
		if _, _, err := runCLI(t, reviewAIArgs(srv, cfg, args...)...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	tag, show, report := (*got)[0], (*got)[1], (*got)[2]
	if b, _ := json.Marshal(tag.body); tag.method != "PUT" || tag.path != "/v1/review-ai/submissions/sub-1/tags" || string(b) != `{"criteriaVersion":"v1","flags":["f1","f2"],"model":"m"}` {
		t.Fatalf("tag %s %s %s", tag.method, tag.path, b)
	}
	if show.method != "GET" || show.path != "/v1/review-ai/submissions/sub-1" {
		t.Fatalf("show %+v", show)
	}
	if report.path != "/v1/review-ai/report" || report.query["from"][0] != "1000" || report.query["to"][0] != "2000" || report.query["criteria"][0] != "v1" {
		t.Fatalf("report %+v", report)
	}
}

func TestReviewServerErrorExitsWithCode(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, _ := reviewAIServer(t, 400, `{"error":"invalid_reason","field":"reasons"}`)
	_, _, err := runCLI(t, reviewAIArgs(srv, cfg, "review", "suggest", "sub-1", "--criteria", "v1", "--model", "m", "--verdict", "reject", "--reason", "nope")...)
	if err == nil || !strings.Contains(err.Error(), "invalid_reason") || !strings.Contains(err.Error(), "reasons") {
		t.Fatalf("error %v", err)
	}
	// Main prints the error to stderr and exits non-zero.
	app, out, errOut := testApp()
	if code := app.Out.Fail(err); code == 0 || out.Len() != 0 || !strings.Contains(errOut.String(), `invalid_reason ({"field":"reasons"})`) {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

// The card-review identity runs unattended with a fixed key: no sign-in, no
// bearer, and the key goes only to the reviewer routes (report stays signed in).
func TestReviewKeyReplacesSignIn(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(EnvReviewKey, "k123")
	var seen []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Clone())
		_, _ = w.Write([]byte(`{"count":0}`))
	}))
	t.Cleanup(srv.Close)
	if _, _, err := runCLI(t, reviewAIArgs(srv, cfg, "review", "queue", "--unshadowed", "--count")...); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0].Get("X-Review-Agent-Key") != "k123" || seen[0].Get("Authorization") != "" {
		t.Fatalf("headers %v", seen)
	}
	seen = nil
	if _, _, err := runCLI(t, reviewAIArgs(srv, cfg, "review", "report")...); err == nil {
		t.Fatal("report without sign-in should fail")
	}
	for _, h := range seen {
		if h.Get("X-Review-Agent-Key") != "" {
			t.Fatalf("key sent to report: %v", h)
		}
	}
}

func TestReviewEvidenceReads(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, got := reviewAIServer(t, 200, `{"ok":true}`)
	cases := []struct {
		args []string
		path string
	}{
		{[]string{"review", "originality", "sub/1"}, "/v1/review-ai/submissions/sub%2F1/originality"},
		{[]string{"review", "author", "sub-1"}, "/v1/review-ai/submissions/sub-1/author"},
		{[]string{"review", "author-cards", "sub-1"}, "/v1/review-ai/submissions/sub-1/author-cards"},
		{[]string{"review", "card", "100123"}, "/v1/review-ai/cards/100123"},
		{[]string{"review", "author-note", "sub-1"}, "/v1/review-ai/submissions/sub-1/author-note"},
	}
	for _, c := range cases {
		*got = nil
		out, _, err := runCLI(t, reviewAIArgs(srv, cfg, c.args...)...)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		if !strings.Contains(out, `"ok": true`) {
			t.Fatalf("%v output %q", c.args, out)
		}
		if len(*got) != 1 || (*got)[0].method != "GET" || (*got)[0].path != c.path || (*got)[0].body != nil {
			t.Fatalf("%v request %+v", c.args, *got)
		}
	}
}

func TestReviewAuthorNoteWrites(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, got := reviewAIServer(t, 200, `{"note":"x","updatedAt":1}`)
	put := func(stdin string, args ...string) map[string]any {
		t.Helper()
		*got = nil
		app, _, _ := testApp()
		app.In = strings.NewReader(stdin)
		root := app.rootCommand()
		root.SetArgs(reviewAIArgs(srv, cfg, append([]string{"review", "author-note", "sub-1"}, args...)...))
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if len(*got) != 1 || (*got)[0].method != "PUT" || (*got)[0].path != "/v1/review-ai/submissions/sub-1/author-note" {
			t.Fatalf("%v request %+v", args, *got)
		}
		return (*got)[0].body
	}
	if b := put("", "--set", "reuses one setting template"); b["note"] != "reuses one setting template" || len(b) != 1 {
		t.Fatalf("set body %v", b)
	}
	if b := put("", "--set", ""); b["note"] != "" {
		t.Fatalf("clear body %v", b)
	}
	if b := put("line one\n{not json}\n", "--file", "-"); b["note"] != "line one\n{not json}\n" {
		t.Fatalf("stdin body %v", b)
	}
}

func TestReviewEvidenceRejectsBadInput(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	srv, got := reviewAIServer(t, 200, `{}`)
	for name, args := range map[string][]string{
		"set and file":  {"review", "author-note", "sub-1", "--set", "a", "--file", "-"},
		"card not num":  {"review", "card", "abc"},
		"card missing":  {"review", "card"},
		"orig two args": {"review", "originality", "a", "b"},
	} {
		if _, _, err := runCLI(t, reviewAIArgs(srv, cfg, args...)...); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("sent %d requests for invalid input", len(*got))
	}
}

func TestReviewKeyOnEvidenceCommands(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(EnvReviewKey, "k123")
	var seen []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Clone())
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	for _, args := range [][]string{
		{"review", "originality", "s"}, {"review", "author", "s"}, {"review", "author-cards", "s"},
		{"review", "card", "100001"}, {"review", "author-note", "s"}, {"review", "author-note", "s", "--set", "n"},
	} {
		if _, _, err := runCLI(t, reviewAIArgs(srv, cfg, args...)...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, h := range seen {
		if h.Get("X-Review-Agent-Key") != "k123" || h.Get("Authorization") != "" {
			t.Fatalf("headers %v", h)
		}
	}
	if len(seen) != 6 {
		t.Fatalf("requests %d", len(seen))
	}
}
