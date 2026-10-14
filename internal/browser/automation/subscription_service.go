package automation

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"reg_go/internal/storage"
)

// 支持的 Kiro 订阅套餐枚举
const (
	PlanKiroPro     = "KIRO_PRO"      // Kiro Pro ($20/mo)
	PlanKiroProPlus = "KIRO_PRO_PLUS" // Kiro Pro+ ($40/mo)
	PlanKiroProMax  = "KIRO_PRO_MAX"  // Kiro Pro Max ($100/mo)
	PlanKiroPower   = "KIRO_POWER"    // Kiro Power ($200/mo)
)

// PlanInfo 套餐信息定义
type PlanInfo struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Price       int    `json:"price"`
	Currency    string `json:"currency"`
	Credits     string `json:"credits"`
	Description string `json:"description"`
	ButtonIndex int    `json:"buttonIndex"`
}

// GetSupportedPlans 获取系统支持的所有订阅套餐列表
func GetSupportedPlans() []PlanInfo {
	return []PlanInfo{
		{
			Type:        PlanKiroPro,
			Title:       "Kiro Pro",
			Price:       20,
			Currency:    "USD",
			Credits:     "1,000",
			Description: "适用于独立开发者日常编程",
			ButtonIndex: 0,
		},
		{
			Type:        PlanKiroProPlus,
			Title:       "Kiro Pro+",
			Price:       40,
			Currency:    "USD",
			Credits:     "2,000",
			Description: "适用于深度高频使用",
			ButtonIndex: 1,
		},
		{
			Type:        PlanKiroProMax,
			Title:       "Kiro Pro Max",
			Price:       100,
			Currency:    "USD",
			Credits:     "5,000",
			Description: "适用于进阶复杂工作负载",
			ButtonIndex: 2,
		},
		{
			Type:        PlanKiroPower,
			Title:       "Kiro Power",
			Price:       200,
			Currency:    "USD",
			Credits:     "10,000",
			Description: "适用于多 Agent 重度自动化集群",
			ButtonIndex: 3,
		},
	}
}

// GetPlanButtonIndex 根据套餐代码获取其在 settings/account 页面对应的 Upgrade 按钮索引 (0-based)
func GetPlanButtonIndex(planType string) int {
	switch strings.ToUpper(strings.TrimSpace(planType)) {
	case PlanKiroProPlus, "PRO+", "PRO_PLUS":
		return 1
	case PlanKiroProMax, "PRO MAX", "PRO_MAX":
		return 2
	case PlanKiroPower, "POWER":
		return 3
	default:
		return 0 // 默认 Kiro Pro
	}
}

// GenerateSubscriptionPaymentURL 为指定 Google 账号生成指定套餐的 Stripe 结账付款链接 (长时效，免二次登录)
func GenerateSubscriptionPaymentURL(acc storage.GoogleAccount, planType string, cb ProgressCallback) (string, error) {
	return GenerateSubscriptionPaymentURLWithLayout(acc, planType, cb, 0, 1)
}

// GenerateSubscriptionPaymentURLWithLayout 带有窗口布局控制的付款链接生成
func GenerateSubscriptionPaymentURLWithLayout(acc storage.GoogleAccount, planType string, cb ProgressCallback, workerIdx, concurrency int) (string, error) {
	report := func(msg string) {
		log.Printf("[SubLink][%s] %s", acc.Email, msg)
		if cb != nil {
			cb(msg)
		}
	}

	if planType == "" {
		planType = PlanKiroPro
	}
	btnIdx := GetPlanButtonIndex(planType)

	winX, winY, winW, winH := CalculateWindowBounds(workerIdx, concurrency)
	report(fmt.Sprintf("正在拉起独立 Chrome 会话 (坐标: [%d, %d], 尺寸: %dx%d)...", winX, winY, winW, winH))

	browserPath, err := FindBrowserPath()
	if err != nil {
		return "", err
	}

	profileDir := acc.ProfileDir
	if profileDir == "" {
		profileDir = storage.GenGoogleProfilePath(acc.Email)
	}
	EnsureProfileUnlocked(profileDir)

	proxyAddr := strings.TrimSpace(acc.Proxy)
	if proxyAddr == "" {
		proxyAddr = strings.TrimSpace(storage.GetProxy())
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserPath),
		chromedp.UserDataDir(profileDir),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-features", "CalculateNativeWinOcclusion,Translate,OptimizationHints,MediaRouter"),
		chromedp.Flag("window-position", fmt.Sprintf("%d,%d", winX, winY)),
		chromedp.Flag("window-size", fmt.Sprintf("%d,%d", winW, winH)),
		chromedp.Flag("headless", false),
		chromedp.Flag("hide-scrollbars", false),
	)

	if proxyAddr != "" {
		opts = append(opts, chromedp.ProxyServer(chromeProxyServer(proxyAddr)))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(automationContext(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	enableProxyAuth(ctx, proxyAddr)

	ctx, cancelTimeout := context.WithTimeout(ctx, 90*time.Second)
	defer cancelTimeout()

	// 监听并捕获 GenerateSubscriptionManagementUrl 请求与响应 RequestID
	reqIDChan := make(chan network.RequestID, 5)

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			if strings.Contains(e.Request.URL, "GenerateSubscriptionManagementUrl") {
				report("🚀 监听到正在请求 GenerateSubscriptionManagementUrl...")
			}
		case *network.EventResponseReceived:
			if strings.Contains(e.Response.URL, "GenerateSubscriptionManagementUrl") {
				report(fmt.Sprintf("📥 收到 GenerateSubscriptionManagementUrl 响应 (状态: %d)", e.Response.Status))
				select {
				case reqIDChan <- e.RequestID:
				default:
				}
			}
		}
	})

	report("正在校验并刷新 Kiro 会话凭据...")
	targetSettingsURL := "https://app.kiro.dev/settings/account"
	if err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate("https://app.kiro.dev/signin?login_provider=Google"),
	); err != nil {
		return "", fmt.Errorf("启动会话失败: %w", err)
	}

	// 状态机循环：处理登录态失效、Cookie 弹窗并点击指定套餐的 Upgrade 按钮
	deadline := time.Now().Add(75 * time.Second)
	ticker := time.NewTicker(1200 * time.Millisecond)
	defer ticker.Stop()

	upgradeClicked := false
	lastClickTime := time.Time{}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("生成订阅链接超时或被取消")
		case reqID := <-reqIDChan:
			time.Sleep(300 * time.Millisecond)
			var body []byte
			_ = chromedp.Run(ctx, chromedp.ActionFunc(func(actCtx context.Context) error {
				var err error
				body, err = network.GetResponseBody(reqID).Do(actCtx)
				return err
			}))
			if len(body) > 0 {
				foundURL := extractStripeCheckoutURL(string(body))
				if foundURL != "" {
					report(fmt.Sprintf("🎉 成功拦截并获取 Stripe 订阅付款链接: %s", truncateURL(foundURL)))
					now := time.Now()
					expiryTime := now.Add(24 * time.Hour).Format("2006-01-02 15:04:05")
					_ = storage.UpdateGoogleAccountPayment(acc.Email, planType, foundURL, expiryTime, "generated")
					CaptureKiroCookiesFromContext(ctx, acc.Email)
					return foundURL, nil
				}
				if strings.Contains(string(body), "UnauthorizedException") || strings.Contains(string(body), "access denied") {
					report("⚠️ Kiro 服务端拒绝生成付款链接: 401 UnauthorizedException (Authentication required or access denied)")
					return "", fmt.Errorf("Kiro 订阅服务拒绝该账号: 401 UnauthorizedException (访问受限或需重新安全验证)")
				}
			}
		case <-ticker.C:
		}

		// 检查是否有前端 window.open 抛出的链接
		var capturedJSURL string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`window.__capturedStripeURL || ''`, &capturedJSURL))
		if strings.Contains(capturedJSURL, "checkout.stripe.com") {
			report(fmt.Sprintf("🎉 通过页面调用成功获取 Stripe 付款链接: %s", truncateURL(capturedJSURL)))
			now := time.Now()
			expiryTime := now.Add(24 * time.Hour).Format("2006-01-02 15:04:05")
			_ = storage.UpdateGoogleAccountPayment(acc.Email, planType, capturedJSURL, expiryTime, "generated")
			CaptureKiroCookiesFromContext(ctx, acc.Email)
			return capturedJSURL, nil
		}

		var curURL string
		_ = chromedp.Run(ctx, chromedp.Location(&curURL))
		uLower := strings.ToLower(curURL)

		// 1. 若掉出登录态到了 signin 页面，快速使用已有的 Google 登录态重定向
		if strings.Contains(uLower, "app.kiro.dev/signin") && !strings.Contains(uLower, "login_provider") {
			report("检测到未进入会话，正在自动拉起 Google 快捷授权...")
			_ = chromedp.Run(ctx, chromedp.Navigate("https://app.kiro.dev/signin?login_provider=Google"))
			time.Sleep(3 * time.Second)
			continue
		}

		// 2. 若在 Google 选号或同意界面，自动点击通过
		if strings.Contains(uLower, "accountchooser") {
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('li, div[role="link"]')?.click()`, nil))
			time.Sleep(2 * time.Second)
			continue
		}
		if strings.Contains(uLower, "signin/oauth") || strings.Contains(uLower, "consent") {
			clickConsentJS := `
(() => {
    const btns = Array.from(document.querySelectorAll('button'));
    for (const b of btns) {
        const t = (b.innerText || '').trim();
        if (t.includes('ดำเนินการต่อ') || t.toLowerCase().includes('continue') || t.toLowerCase().includes('allow') || t.includes('继续')) {
            b.click();
            return true;
        }
    }
    return false;
})()
`
			_ = chromedp.Run(ctx, chromedp.Evaluate(clickConsentJS, nil))
			time.Sleep(3 * time.Second)
			continue
		}

		// 3. 若已回到 /home 或根路径，重定向至 /settings/account
		if strings.Contains(uLower, "app.kiro.dev/home") ||
			(strings.Contains(uLower, "app.kiro.dev") &&
				!strings.Contains(uLower, "settings") &&
				!strings.Contains(uLower, "signin") &&
				!strings.Contains(uLower, "login") &&
				!strings.Contains(uLower, "oauth")) {
			_ = chromedp.Run(ctx, chromedp.Navigate(targetSettingsURL))
			time.Sleep(2 * time.Second)
			continue
		}

		// 4. 若在 /settings/account 页面
		if strings.Contains(uLower, "settings/account") {
			// A. 先关闭 Cookie 弹窗 (消除遮罩)
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('.awsccc-u-btn-primary')?.click()`, nil))

			// 每隔 3.5 秒若未进入 Stripe 或未捕获链接，重试触发点击
			if !upgradeClicked || time.Since(lastClickTime) > 3500*time.Millisecond {
				clickBtnJS := fmt.Sprintf(`
(() => {
    let upgradeBtns = Array.from(document.querySelectorAll('button._upgradeButton_11kdv_212'));
    if (upgradeBtns.length === 0) {
        upgradeBtns = Array.from(document.querySelectorAll('button')).filter(b => {
            const t = (b.innerText || '').trim().toLowerCase();
            return t === 'upgrade' || t === '升级';
        });
    }
    const targetIdx = %d;
    if (upgradeBtns.length > targetIdx) {
        const btn = upgradeBtns[targetIdx];
        const rect = btn.getBoundingClientRect();
        if (rect.width > 0 && rect.height > 0) {
            btn.scrollIntoView({ behavior: 'smooth', block: 'center' });
            const opts = { bubbles: true, cancelable: true, view: window, clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2 };
            btn.dispatchEvent(new MouseEvent('mousedown', opts));
            btn.dispatchEvent(new MouseEvent('mouseup', opts));
            btn.dispatchEvent(new MouseEvent('click', opts));
            btn.click();
            return 'clicked';
        }
    }
    return 'btns_found_' + upgradeBtns.length;
})()
`, btnIdx)
				var clickRes string
				_ = chromedp.Run(ctx, chromedp.Evaluate(clickBtnJS, &clickRes))
				if clickRes == "clicked" {
					upgradeClicked = true
					lastClickTime = time.Now()
					report(fmt.Sprintf("已定位并仿真触发套餐 [%s] 对应的 Upgrade 按钮，等待 Stripe 链接下发...", planType))
				}
			}
		}
	}

	// 最终检查 channel 是否有收到
	select {
	case reqID := <-reqIDChan:
		var body []byte
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(actCtx context.Context) error {
			var err error
			body, err = network.GetResponseBody(reqID).Do(actCtx)
			return err
		}))
		if len(body) > 0 {
			foundURL := extractStripeCheckoutURL(string(body))
			if foundURL != "" {
				expiryTime := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
				_ = storage.UpdateGoogleAccountPayment(acc.Email, planType, foundURL, expiryTime, "generated")
				return foundURL, nil
			}
		}
	default:
	}

	return "", fmt.Errorf("未能生成套餐 [%s] 的付款链接 (超时或账户无对应权限)", planType)
}

// BatchGenerateSubscriptionLinks 并发批量为多个 Google 账号生成订阅付款链接
func BatchGenerateSubscriptionLinks(accounts []storage.GoogleAccount, planType string, concurrency int, cb ProgressCallback) (map[string]string, error) {
	if concurrency <= 0 {
		concurrency = 2
	}
	if concurrency > 5 {
		concurrency = 5
	}

	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	var circuitBroken atomic.Bool
	var consecutiveRapidFailures atomic.Int32

	for i, acc := range accounts {
		if circuitBroken.Load() {
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		workerIdx := i % concurrency
		go func(target storage.GoogleAccount, wIdx int) {
			defer wg.Done()
			defer func() { <-sem }()

			if circuitBroken.Load() {
				return
			}

			startT := time.Now()
			url, err := GenerateSubscriptionPaymentURLWithLayout(target, planType, cb, wIdx, concurrency)
			mu.Lock()
			if err == nil && url != "" {
				results[target.Email] = url
				consecutiveRapidFailures.Store(0)
			} else if err != nil {
				results[target.Email] = "error: " + err.Error()
				// 如果在 6 秒内快速报错（如代理失效、网络阻断、ERR_PROXY_CONNECTION_FAILED），累计快速失败计数
				if time.Since(startT) < 6*time.Second {
					if consecutiveRapidFailures.Add(1) >= 3 {
						if circuitBroken.CompareAndSwap(false, true) {
							if cb != nil {
								cb("⚠️ 检测到连续 3 个账号极速启动失败，疑似代理或网络故障，已自动熔断终止后续批量任务！")
							}
						}
					}
				}
			}
			mu.Unlock()
		}(acc, workerIdx)
	}

	wg.Wait()
	return results, nil
}

// extractStripeCheckoutURL 从 RPC 响应体中正则提取 Stripe Checkout URL
func extractStripeCheckoutURL(body string) string {
	re := regexp.MustCompile(`https://checkout\.stripe\.com/c/pay/[a-zA-Z0-9_\-\#\%\.\=]+`)
	matches := re.FindAllString(body, -1)
	if len(matches) > 0 {
		raw := matches[0]
		// 校验 URL 合法性
		if _, err := url.Parse(raw); err == nil {
			return raw
		}
	}
	return ""
}

func truncateURL(u string) string {
	if len(u) <= 60 {
		return u
	}
	return u[:55] + "..."
}
