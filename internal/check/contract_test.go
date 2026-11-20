package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Compare the embedded SDK with the author's published contract. An internal
// Stage checkout can also validate a candidate before the contract is published.
func TestSandboxSDKContract(t *testing.T) {
	path := os.Getenv("HEARTHROOM_SANDBOX_CONTRACT")
	if path == "" {
		root := os.Getenv("HEARTHROOM_STAGE_SOURCE")
		if root == "" {
			t.Skip("set HEARTHROOM_SANDBOX_CONTRACT to the public contract file")
		}
		path = filepath.Join(root, "contract", "sandbox-contract.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stage Contract
	if err := json.Unmarshal(raw, &stage); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract.SDK, stage.SDK) || !reflect.DeepEqual(contract.Events, stage.Events) {
		t.Fatal("embedded SDK keys, capabilities, properties or events differ from the author contract; copy the public sandbox contract to internal/check/sandbox-contract.json")
	}
}
