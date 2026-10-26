package cli

import (
	"bytes"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/sync"
	"strings"
	"testing"
)

func TestPrintPushRewrittenRuleSizes(t *testing.T) {
	var out bytes.Buffer
	a := &App{Out: output.Printer{Out: &out, Err: &out}}
	a.printPush(&sync.PushResult{DryRun: true, Target: "owned", Changed: []string{"authorAsset"}, Rules: []sync.RuleSize{{Index: 1, ID: "panel", Name: "Panel", RewrittenBytes: 131072, LimitBytes: 131072, RemainingBytes: 0}}, RulesTotal: &sync.RuleTotalSize{Bytes: 131080, LimitBytes: 33554432, RemainingBytes: 33423352}})
	for _, want := range []string{"after URL rewriting (UTF-8 bytes)", `Rule 1 ("panel", "Panel"): 131072 of 131072 bytes; 0 bytes remaining`, "Rules total (find + replacement + name): 131080 of 33554432 bytes", "sections to send:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}
