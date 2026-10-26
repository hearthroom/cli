package cli

import (
	"bytes"
	"encoding/json"
	"github.com/hearthroom/cli/internal/output"
	"github.com/hearthroom/cli/internal/sync"
	"strings"
	"testing"
)

func TestPrintPushIncludesConfirmedVisibility(t *testing.T) {
	var res sync.PushResult
	json.Unmarshal([]byte(`{"created":true,"target":"owned","roleId":"fixture","roleVisibility":"unlisted"}`), &res)
	var out bytes.Buffer
	app := &App{Out: output.Printer{Out: &out, Err: &out}}
	app.printPush(&res)
	if !strings.Contains(out.String(), "unlisted") {
		t.Fatalf("missing visibility: %s", out.String())
	}
}
