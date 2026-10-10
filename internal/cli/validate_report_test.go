package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hearthroom/cli/internal/sync"
)

// The budget lists the card page introduction under the summary with its own limit.
func TestReportShowsIntroBudget(t *testing.T) {
	var r sync.Report
	raw := `{"status":"pass","tokenBudget":{"roleDescChars":12,"roleReadmeChars":340,"limits":{"roleDescMaxChars":200,"roleReadmeMaxChars":20000}}}`
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	app, out, _ := testApp()
	app.printReport(&r)
	lines := strings.Split(out.String(), "\n")
	for i, l := range lines {
		if strings.Contains(l, "summary") {
			if i+1 >= len(lines) || !strings.Contains(lines[i+1], "intro") || !strings.Contains(lines[i+1], "340 / 20000") {
				t.Fatalf("report:\n%s", out.String())
			}
			return
		}
	}
	t.Fatalf("no summary row:\n%s", out.String())
}
