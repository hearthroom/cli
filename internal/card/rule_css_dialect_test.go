package card

import (
	"encoding/json"
	"strings"
	"testing"
)

// cssDialect（規則裡 <style> 的寫法）在網站與匯入時設定；pull 下來再 push 回去要原樣保留，沒填的不憑空長出來。
func TestRuleCSSDialectSurvivesPullAndPush(t *testing.T) {
	var asset RemoteAsset
	if err := json.Unmarshal([]byte(`{"rules":[
		{"id":"a","find":"x","replace":"<style>#ai-chat-input{color:red}</style>","enabled":true,"cssDialect":"fengyue"},
		{"id":"b","find":"x","replace":"y","enabled":true}
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
	if !strings.Contains(got, `"cssDialect":"fengyue"`) || strings.Count(got, `"cssDialect"`) != 1 {
		t.Fatalf("push changed cssDialect: %s", got)
	}
}
