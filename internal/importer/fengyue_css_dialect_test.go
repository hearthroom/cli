package importer

import "testing"

// 風月的內置 CSS 匯進來要標成風月寫法，畫布才會把它對準風月介面元素的部分轉到我們的元素上。
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
			if r.CardCSS != "#ai-chat-input{color:red}" || r.CardCSSDialect != "fengyue" || len(r.Rules.Rules) != 0 {
				t.Fatalf("card CSS missing or still carried by a fake rule: %+v", r)
			}
		})
	}
}
