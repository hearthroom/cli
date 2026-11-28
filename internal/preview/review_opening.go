package preview

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// ReviewOpeningScreenshot opens an offline preview in a fresh temporary browser profile.
func ReviewOpeningScreenshot(ctx context.Context, document string) ([]byte, error) {
	chrome, err := FindChrome()
	if err != nil {
		return nil, err
	}
	ctx, cancelTimeout := context.WithTimeout(ctx, 25*time.Second)
	defer cancelTimeout()
	alloc, cancelAlloc := allocator(ctx, chrome)
	defer cancelAlloc()
	tab, cancelTab := chromedp.NewContext(alloc)
	defer cancelTab()
	location := "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(document))
	if err := chromedp.Do(tab,
		chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
			if _, err := cdp.Call(ctx, target, network.Enable, network.EnableParams{}); err != nil {
				return err
			}
			if _, err := cdp.Call(ctx, target, network.EmulateNetworkConditionsByRule, network.EmulateNetworkConditionsByRuleParams{
				MatchedNetworkConditions: []*network.Conditions{{Offline: true}},
			}); err != nil {
				return err
			}
			_, err := cdp.Call(ctx, target, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
			return err
		}),
		chromedp.EmulateViewport(960, 900),
		chromedp.Navigate(location),
		chromedp.WaitReady("body"),
	); err != nil {
		return nil, err
	}
	return chromedp.Run(tab, chromedp.CaptureScreenshot())
}
