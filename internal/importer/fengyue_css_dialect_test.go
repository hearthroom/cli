package importer

import "testing"

// 新匯入保留 v2 提示，讓畫布套用原站基底；舊資料的提示不在這條路徑升版。
// 內置 CSS 空著、只有 custom_css 時拿 custom_css。
func TestFengyueCardCSSRetainsImportHint(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  map[string]any
	}{
		{"builtInCss", map[string]any{"title": "T", "pre_prompt": "p", "ai_variable_prompt": "v", "builtInCss": "#ai-chat-input{color:red}", "custom_css": ""}},
		{"custom_css fallback", map[string]any{"title": "T", "pre_prompt": "p", "ai_variable_prompt": "v", "builtInCss": "", "custom_css": "#ai-chat-input{color:red}"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := FromFengyue(tc.raw, "zh-Hant")
			if r.Rules == nil {
				t.Fatal("no rules")
			}
			if r.CardCSS != "#ai-chat-input{color:red}" || r.CardCSSDialect != "fengyue-v2" || len(r.Rules.Rules) != 0 {
				t.Fatalf("card CSS missing or still carried by a fake rule: %+v", r)
			}
		})
	}
}
