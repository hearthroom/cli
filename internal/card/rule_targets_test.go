package card

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDisplayRuleTargetsSurviveFolderAndPayload(t *testing.T) {
	for _, rawTargets := range []string{"", `,"targets":[]`, `,"targets":["ai"]`, `,"targets":["user","system"]`} {
		t.Run(rawTargets, func(t *testing.T) {
			f := sample(t)
			raw := []byte(`{"rules":[{"id":"r","find":"x","replace":"y","enabled":true` + rawTargets + `}]}`)
			if err := os.WriteFile(filepath.Join(f.Dir, RulesFile), raw, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(f.Dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := loaded.Save(); err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(filepath.Join(f.Dir, RulesFile))
			if err != nil {
				t.Fatal(err)
			}
			payload, err := loaded.Build(nil)
			if err != nil {
				t.Fatal(err)
			}
			pushed, err := json.Marshal(payload.AuthorAsset)
			if err != nil {
				t.Fatal(err)
			}
			var expected struct {
				Rules []map[string]any `json:"rules"`
			}
			json.Unmarshal(raw, &expected)
			for label, body := range map[string][]byte{"saved": saved, "push": pushed} {
				var got struct {
					Rules []map[string]any `json:"rules"`
				}
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				want, wantOK := expected.Rules[0]["targets"]
				value, gotOK := got.Rules[0]["targets"]
				if wantOK != gotOK || !reflect.DeepEqual(want, value) {
					t.Fatalf("%s targets=%#v present=%v, want %#v present=%v", label, value, gotOK, want, wantOK)
				}
			}
			var remote RemoteAsset
			if err := json.Unmarshal(raw, &remote); err != nil {
				t.Fatal(err)
			}
			pulled := FromRemote(t.TempDir(), RemoteDetail{RoleID: "r", RoleName: "Fixture"}, "", "", nil, &remote)
			if err := pulled.Save(); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(pulled.Dir, RulesFile))
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Rules []map[string]any `json:"rules"`
			}
			json.Unmarshal(body, &got)
			if !reflect.DeepEqual(expected.Rules[0]["targets"], got.Rules[0]["targets"]) {
				t.Fatalf("pull targets=%#v want %#v", got.Rules[0]["targets"], expected.Rules[0]["targets"])
			}
		})
	}
}
