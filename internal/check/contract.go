// Package check runs the local checks of a card folder before a push: the
// display rules against the sandbox author contract, the markers the model is
// told to write against the rules that draw them, and the things the provider's
// validate and render cannot see (a pattern that matches the empty string, an
// attribute the sanitizer strips, a save key that is not a key).
//
// Every fact used here comes from sandbox-contract.json, which the chat page's
// repository generates from its source and hearthroom/skills vendors; keep the
// embedded copy in step with scripts/sandbox-contract.json there.
package check

import (
	_ "embed"
	"encoding/json"
)

//go:embed sandbox-contract.json
var contractJSON []byte

// Contract is the subset of the generated sandbox contract these checks read.
type Contract struct {
	SDK struct {
		Keys         []string `json:"keys"`
		Capabilities []string `json:"capabilities"`
		NotProvided  []string `json:"notProvided"`
	} `json:"sdk"`
	Save struct {
		KeyPattern string `json:"keyPattern"`
	} `json:"save"`
	Events struct {
		Names []string `json:"names"`
	} `json:"events"`
	Rules struct {
		Flags string `json:"flags"`
	} `json:"rules"`
	Provider struct {
		ReplaceMaxBytes int      `json:"replaceMaxBytes"`
		TotalMaxBytes   int      `json:"totalMaxBytes"`
		MountLayer      []string `json:"mountLayer"`
		PageMode        []string `json:"pageMode"`
		CardFormat      []string `json:"cardFormat"`
	} `json:"provider"`
}

var contract = mustContract()

func mustContract() Contract {
	var c Contract
	if err := json.Unmarshal(contractJSON, &c); err != nil {
		panic("check: embedded sandbox-contract.json is invalid: " + err.Error())
	}
	return c
}

// ContractFacts returns the embedded contract (for tests and `--json` consumers).
func ContractFacts() Contract { return contract }

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
