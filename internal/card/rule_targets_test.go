package card

import (
	"encoding/json"
	"strings"
	"testing"
)

// Rule targets (ai/user/system) are set on the site and by imports. A pulled rule must
// keep them on push; missing means every message and [] means none, so the two stay apart.
func TestRuleTargetsSurvivePullAndPush(t *testing.T) {
	var asset RemoteAsset
	if err := json.Unmarshal([]byte(`{"rules":[
		{"id":"a","find":"x","replace":"y","enabled":true,"targets":["ai","user"]},
		{"id":"b","find":"x","replace":"y","enabled":true,"targets":[]},
		{"id":"c","find":"x","replace":"y","enabled":true}
	]}`), &asset); err != nil {
		t.Fatal(err)
	}
	f := FromRemote(t.TempDir(), RemoteDetail{RoleName: "Mira", RoleWelcome: "hi"}, "", "", nil, &asset)
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := loaded.Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(payload.AuthorAsset["rules"])
	got := string(raw)
	for _, want := range []string{`"targets":["ai","user"]`, `"targets":[]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("push lost %s: %s", want, got)
		}
	}
	if strings.Count(got, `"targets"`) != 2 {
		t.Fatalf("a rule without targets must stay without them: %s", got)
	}
}
