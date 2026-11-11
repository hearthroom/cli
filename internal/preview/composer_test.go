package preview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Use the real bundled shell and native input: its standard composer deliberately
// rejects untrusted script clicks. Each run owns its browser profile and server.
func TestPreviewCardCSSComposer(t *testing.T) {
	if os.Getenv("HEARTHROOM_TEST_CHROME") != "1" || os.Getenv("HEARTHROOM_TEST_SHELL") == "" {
		t.Skip("set HEARTHROOM_TEST_CHROME=1 and HEARTHROOM_TEST_SHELL to run")
	}
	chrome, err := FindChrome()
	if err != nil {
		t.Fatal(err)
	}
	profile := t.TempDir()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(chrome), chromedp.UserDataDir(profile))
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	browser, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()
	if _, err := chromedp.Run(browser, chromedp.Evaluate[int](`1`)); err != nil {
		t.Fatal(err)
	}
	t.Logf("owned Chrome pid=%d profile=%s", chromedp.FromContext(browser).Browser.Process().Pid, profile)
	// Existing Stage composer vocabulary, including accessible names. The query
	// uses the shell's existing locale selection; no CLI language flag is added.
	localized := map[string][]string{
		"zh-Hant": {"說點什麼⋯", "停止", "更多", "送出", "貼上", "清空", "每輪點數 · 模型設定", "不知道怎麼回？讓 AI 幫你起個頭"},
		"zh-Hans": {"说点什么⋯", "停止", "更多", "发送", "粘贴", "清空", "每轮点数 · 模型设置", "不知道怎么回？让 AI 帮你起个头"},
		"en":      {"Say something…", "Stop", "More", "Send", "Paste", "Clear", "Credits per turn · Model", "Stuck? Let AI draft a reply"},
		"ja":      {"何か話しかけて⋯", "停止", "その他", "送信", "貼り付け", "クリア", "1 ターンあたりのポイント · モデル設定", "返事に迷ったら AI に下書きしてもらう"},
		"ko":      {"무엇이든 말해 보세요⋯", "중지", "더보기", "보내기", "붙여넣기", "지우기", "1턴당 포인트 · 모델 설정", "뭐라고 답할지 막히면 AI에게 초안을 맡기세요"},
	}
	localeCases := map[string]string{
		"locale-zh-Hant": "zh-Hant", "locale-zh-Hans": "zh-Hans", "locale-en": "en", "locale-ja": "ja", "locale-ko": "ko",
		"locale-zh-TW": "zh-Hant", "locale-zh_CN": "zh-Hans", "locale-ja-JP": "ja", "locale-ko_KR": "ko", "locale-unknown": "en",
	}

	for _, tc := range []struct {
		name, css, query, mode string
		present, interact      bool
		width, height          int64
	}{
		{"legacy", "", "", "shell", false, false, 1280, 800},
		{"legacy-rules-off", "", "&rules=off", "shell", false, false, 390, 844},
		{"empty-file", "", "", "standard", true, false, 390, 844},
		{"file-rules-off", "#ai-chat-input{background:rgb(40,90,130)}", "&rules=off", "standard", true, false, 390, 844},
		{"hint-desktop", "#ai-chat-input{background:rgb(40,90,130)}", "", "standard", true, true, 1280, 800},
		{"hint-phone", "#ai-chat-input{background:rgb(40,90,130)}", "", "standard", true, true, 390, 844},
		{"hint-desktop-light", "#ai-chat-input{background:rgb(40,90,130)}", "", "standard", true, true, 1280, 800},
		{"hint-phone-light", "#ai-chat-input{background:rgb(40,90,130)}", "", "standard", true, true, 390, 844},
		{"locale-zh-Hant", "", "&locale=zh-Hant", "standard", true, false, 390, 844},
		{"locale-zh-Hans", "", "&locale=zh-Hans", "standard", true, false, 390, 844},
		{"locale-en", "", "&locale=en", "standard", true, false, 390, 844},
		{"locale-ja", "", "&locale=ja", "standard", true, false, 390, 844},
		{"locale-ko", "", "&locale=ko", "standard", true, false, 390, 844},
		{"locale-zh-TW", "", "&locale=zh-TW", "standard", true, false, 390, 844},
		{"locale-zh_CN", "", "&locale=zh_CN", "standard", true, false, 390, 844},
		{"locale-ja-JP", "", "&locale=ja-JP", "standard", true, false, 390, 844},
		{"locale-ko_KR", "", "&locale=ko_KR", "standard", true, false, 390, 844},
		{"locale-unknown", "", "&locale=xx-YY", "standard", true, false, 390, 844},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card := t.TempDir()
			files := map[string]string{
				"card.json": `{"name":"Preview fixture"}`, "welcome.md": "Hello.",
				"rules.json": `{"pageMode":"sandbox","cardCssDialect":"fengyue","rules":[]}`,
			}
			if tc.present {
				files["card.css"] = tc.css
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(card, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			srv, err := Listen(os.Getenv("HEARTHROOM_TEST_SHELL"), card, 0, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer srv.Close()
			tab, cancelTab := chromedp.NewContext(browser)
			defer cancelTab()
			ctx, cancel := context.WithTimeout(tab, 45*time.Second)
			defer cancel()
			if err := chromedp.Do(ctx, chromedp.EmulateViewport(tc.width, tc.height), chromedp.Navigate(srv.URL+"&bare=1"+tc.query)); err != nil {
				t.Fatal(err)
			}
			mustWait := func(expr string) {
				t.Helper()
				if err := waitFor(ctx, expr, 5*time.Second); err != nil {
					t.Fatal(err)
				}
			}
			mustWait(`typeof __preview === 'object' && __preview.sent.some(m=>m.type==='ready')`)
			const doc = `document.getElementById('frame').contentDocument`
			fg, bg := "rgb(255, 255, 255)", "rgb(23, 24, 26)"
			if strings.HasSuffix(tc.name, "-light") {
				if _, err := chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void](`const s=document.getElementById('theme');s.value='light';s.dispatchEvent(new Event('change'))`)); err != nil {
					t.Fatal(err)
				}
				fg, bg = "rgb(33, 34, 38)", "rgb(255, 255, 255)"
			}
			mode, err := chromedp.Run(ctx, chromedp.Evaluate[string](doc+`.querySelector('[data-chat="root"]').getAttribute('data-chrome')`))
			if err != nil || mode != tc.mode {
				t.Fatalf("chrome mode=%q want %q: %v", mode, tc.mode, err)
			}
			click := func(selector string) {
				t.Helper()
				point, err := chromedp.Run(ctx, chromedp.Evaluate[[]float64](`(()=>{const f=document.getElementById('frame'),d=f.contentDocument;const e=Array.from(d.querySelectorAll(`+jsString(selector)+`)).find(e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&!e.hidden});if(!e)throw Error('missing visible '+`+jsString(selector)+`);const a=f.getBoundingClientRect(),b=e.getBoundingClientRect();return [a.left+b.left+b.width/2,a.top+b.top+b.height/2]})()`))
				if err != nil {
					t.Fatal(err)
				}
				if err := chromedp.Do(ctx, chromedp.MouseClickXY(point[0], point[1])); err != nil {
					t.Fatal(err)
				}
			}
			// A synthetic empty payload for rules-off must never opt old cards in;
			// their native composer still uses the original message.send request.
			if !tc.present {
				click(`#send_textarea`)
				if err := chromedp.Do(ctx, chromedp.KeyEvent("Legacy turn")); err != nil {
					t.Fatal(err)
				}
				click(`#send_but`)
				mustWait(`__preview.sent.some(m=>m.type==='request'&&m.op==='message.send'&&m.args[0]==='Legacy turn') && !__preview.state.busy && __preview.state.messages.some(m=>m.content==='OK.')`)
				return
			}
			lang := localeCases[tc.name]
			if lang == "" {
				lang = "zh-Hant"
			}
			labels, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](`(()=>{const d=`+doc+`;const q=s=>d.querySelector(s);return [q('.chat-input-collapsed-placeholder').textContent,q('#mes_stop').getAttribute('aria-label'),q('#options_button').getAttribute('aria-label'),q('.lt-send').getAttribute('aria-label'),q('[data-chat="paste"]').textContent.trim(),q('[data-chat="clear"]').textContent.trim(),q('.mind-type').getAttribute('aria-label'),q('.ai-assistant').getAttribute('aria-label')]})()`))
			if err != nil || strings.Join(labels, "\x00") != strings.Join(localized[lang], "\x00") {
				t.Fatalf("composer locale %s: got %q want %q: %v", lang, labels, localized[lang], err)
			}
			if !tc.interact {
				return
			}
			mustWait(`getComputedStyle(` + doc + `.querySelector('.composer-field')).backgroundColor==='rgb(40, 90, 130)'`)
			mustWait(`getComputedStyle(` + doc + `.querySelector('[data-chat="message-body"]')).color===` + jsString(fg) + ` && getComputedStyle(` + doc + `.querySelector('.header-roleName')).color===` + jsString(fg) + ` && getComputedStyle(` + doc + `.querySelector('[data-chat="root"]')).backgroundColor===` + jsString(bg))
			mustWait(doc + `.querySelector('.ai-assistant.is-off') !== null && !` + doc + `.querySelector('[data-lt="back"], [data-lt="favorite"], [data-lt="comments"], [data-lt="fullscreen"]')`)
			input := func(text string) {
				t.Helper()
				click(`.chat-input-collapsed-display, #send_textarea:not([aria-hidden="true"])`)
				if err := chromedp.Do(ctx, chromedp.KeyEvent(text)); err != nil {
					t.Fatal(err)
				}
				mustWait(`__preview.sent.some(m=>m.type==='input'&&m.value===` + jsString(text) + `)`)
				click(`.lt-send`)
			}
			input("Native preview turn")
			mustWait(`__preview.state.messages.some(m=>m.role==='user'&&m.content==='Native preview turn') && __preview.state.busy`)
			mustWait(`!__preview.state.busy && __preview.state.messages.some(m=>m.role==='ai'&&m.content==='OK.')`)
			mustWait(doc + `.querySelector('#send_textarea').value==='' && ` + doc + `.querySelector('#mes_stop').hidden`)
			input("Stop before first word")
			mustWait(doc + `.querySelector('#mes_stop').hidden===false`)
			click(`#mes_stop`)
			mustWait(`!__preview.state.busy && __preview.state.messages.length===5 && __preview.state.messages[4].content===''`)

			// Stop a real writing stream, then send again. A late task from the
			// canceled stream must neither append its full reply nor settle twice.
			if _, err := chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void](`__preview.state.samples=[`+jsString(strings.Repeat("Long reply ", 100))+`]`)); err != nil {
				t.Fatal(err)
			}
			input("Stop this turn")
			mustWait(`__preview.state.busy && ` + doc + `.querySelectorAll('[data-chat="message-body"]')[6]?.textContent.includes('Long reply')`)
			click(`#mes_stop`)
			mustWait(`!__preview.state.busy`)
			if _, err := chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void](`__preview.state.samples=['After stop']`)); err != nil {
				t.Fatal(err)
			}
			input("Next turn")
			mustWait(`!__preview.state.busy && __preview.state.messages.length===9 && __preview.state.messages[8].content==='After stop'`)
			ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`__preview.state.messages[6].content.length>0 && __preview.state.messages[6].content.length<1100 && new Set(__preview.state.messages.map(m=>m.id)).size===9`))
			if err != nil || !ok {
				detail, _ := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify(__preview.state.messages)`))
				t.Fatalf("canceled reply not preserved exactly once: %v %s", err, detail)
			}
			input("Reload while preparing")
			mustWait(`__preview.state.busy`)
			click(`.chat-input-collapsed-display, #send_textarea:not([aria-hidden="true"])`)
			if err := chromedp.Do(ctx, chromedp.KeyEvent("Unsent draft")); err != nil {
				t.Fatal(err)
			}
			mustWait(`__preview.state.inputValue==='Unsent draft'`)
			if _, err := chromedp.Run(ctx, chromedp.Evaluate[chromedp.Void](`window.readyBeforeReload=__preview.sent.filter(m=>m.type==='ready').length;__preview.reload()`)); err != nil {
				t.Fatal(err)
			}
			mustWait(`__preview.sent.filter(m=>m.type==='ready').length>window.readyBeforeReload && !__preview.state.busy && __preview.state.messages.length===1 && __preview.state.inputValue===''`)
			input("After reload")
			mustWait(`!__preview.state.busy && __preview.state.messages.length===3 && __preview.state.messages[1].content==='After reload'`)
			if shots := os.Getenv("HEARTHROOM_TEST_SHOTS"); shots != "" {
				png, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(shots, fmt.Sprintf("%s.png", tc.name)), png, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
