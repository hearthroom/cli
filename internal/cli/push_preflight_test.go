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
	a.printPush(&sync.PushResult{DryRun: true, Target: "owned", Changed: []string{"authorAsset"}, Rules: []sync.RuleSize{{Index: 1, ID: "panel", Name: "Panel", RewrittenBytes: 524288, LimitBytes: 524288, RemainingBytes: 0}}, RulesTotal: &sync.RuleTotalSize{Bytes: 524296, LimitBytes: 33554432, RemainingBytes: 33030136}})
	for _, want := range []string{"after URL rewriting (UTF-8 bytes)", `Rule 1 ("panel", "Panel"): 524288 of 524288 bytes; 0 bytes remaining`, "Rules total (find + replacement + name): 524296 of 33554432 bytes", "sections to send:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
}
