package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/auth"
)

// A memory summary is billed apart from its turn. The wallet names it and the
// turn's operation (the streamId a play turn reports) instead of a bare "chat".
func TestWalletNamesMemoryCompaction(t *testing.T) {
	cfg := authTestEnv(t)
	t.Setenv(auth.EnvToken, "tok")
	mux := http.NewServeMux()
	mux.HandleFunc("/open/v1/me/wallet", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"available":900,"ledger":[
			{"at":"2026-10-30T12:24:40Z","amount":-88,"reason":"chat","memoryCompaction":true,"operationId":"01a18cc4-0000-7000-8000-000000000001"},
			{"at":"2026-10-30T12:24:30Z","amount":-797,"reason":"chat"}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	out, errOut, err := runCLI(t, "wallet", "--api", srv.URL, "--config-dir", cfg)
	if err != nil {
		t.Fatalf("wallet: %v %s", err, errOut)
	}
	if !strings.Contains(out, "-88  memory compaction (turn 01a18cc4-0000-7000-8000-000000000001)") || !strings.Contains(out, "-797  chat") {
		t.Fatalf("wallet output:\n%s", out)
	}
}
