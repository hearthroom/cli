package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Stage generates this contract from createSdk's runtime object. CI checks its
// latest main so a newly added capability cannot silently leave the CLI behind.
func TestSandboxSDKContract(t *testing.T) {
	root := os.Getenv("HEARTHROOM_STAGE_SOURCE")
	if root == "" {
		t.Skip("set HEARTHROOM_STAGE_SOURCE to a stage checkout for SDK contract verification")
	}
	raw, err := os.ReadFile(filepath.Join(root, "contract", "sandbox-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stage Contract
	if err := json.Unmarshal(raw, &stage); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract.SDK, stage.SDK) || !reflect.DeepEqual(contract.Events, stage.Events) {
		t.Fatal("embedded SDK keys, capabilities, properties or events differ from stage; copy stage's generated contract/sandbox-contract.json to internal/check/sandbox-contract.json")
	}
}
