package automation

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"reg_go/internal/storage"
)

// ProgressCallback 用于向前端推送当前自动化登录进度的回调
type ProgressCallback func(message string)

// CalculateWindowBounds 根据当前 Worker 索引与总并发数计算出最优屏幕位置与尺寸 (多窗口智能平铺与错位排版)
func CalculateWindowBounds(workerIdx, concurrency int) (x, y, w, h int) {
	if concurrency <= 1 {
		return 100, 50, 1200, 850
	}

	switch concurrency {
	case 2:
		w = 930
		h = 880
		if workerIdx%2 == 0 {
			x, y = 20, 35
		} else {
			x, y = 970, 35
		}
	case 3:
		w = 930
		h = 470
		switch workerIdx % 3 {
		case 0:
			x, y = 20, 30
		case 1:
			x, y = 970, 30
		case 2:
			x, y = 495, 535
		}
	case 4:
		w = 930
		h = 470
		switch workerIdx % 4 {
		case 0:
			x, y = 20, 30
		case 1:
			x, y = 970, 30
		case 2:
			x, y = 20, 535
		case 3:
			x, y = 970, 535
		}
	default: // 5及以上并发：阶梯错位瀑布流 (Cascade)
		w = 960
		h = 700
		slot := workerIdx % 7
		x = 30 + slot*55
		y = 30 + slot*45
	}
	return x, y, w, h
}

// AutoLoginGoogleAccount 驱动独立 Chrome Profile 全自动登录 Google 账号 (默认单窗口)
func AutoLoginGoogleAccount(acc storage.GoogleAccount, autoAuthorizeKiro bool, cb ProgressCallback) error {
	return AutoLoginGoogleAccountWithLayout(acc, autoAuthorizeKiro, cb, 0, 1)
}

// AutoLoginGoogleAccountWithLayout 带有智能窗口排版和并发调度控制的自动化登录执行入口
// workerIdx: 当前并发 Worker 序号 (用于计算窗口错位坐标)
// concurrency: 本次任务总并发数 (用于计算四宫格或左右平铺布局)
func AutoLoginGoogleAccountWithLayout(acc storage.GoogleAccount, autoAuthorizeKiro bool, cb ProgressCallback, workerIdx, concurrency int) error {
	report := func(msg string) {
		log.Printf("[AutoLogin][%s] %s", acc.Email, msg)
		if cb != nil {
			cb(msg)
		}
	}

	winX, winY, winW, winH := CalculateWindowBounds(workerIdx, concurrency)
	report(fmt.Sprintf("正在初始化独立 Chrome 环境 (窗口网格坐标: [%d, %d], 尺寸: %dx%d)...", winX, winY, winW, winH))

	browserPath, err := FindBrowserPath()
	if err != nil {
		return err
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

	// 构造防指纹识别与环境隔离的 chromedp 执行参数
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserPath),
		chromedp.UserDataDir(profileDir),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("password-store", "basic"),
		// 彻底关闭 Chrome 后台与被遮挡降频，保证多窗口并发时 100% 满血调度
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-features", "CalculateNativeWinOcclusion,Translate,OptimizationHints,MediaRouter"),
		// 动态屏幕排版定位与尺寸
		chromedp.Flag("window-position", fmt.Sprintf("%d,%d", winX, winY)),
		chromedp.Flag("window-size", fmt.Sprintf("%d,%d", winW, winH)),
		// 必须采用有头模式以规避 Google 严格的 Headless 检测，同时保证异常时用户可前台辅助
		chromedp.Flag("headless", false),
		chromedp.Flag("hide-scrollbars", false),
		chromedp.Flag("enable-automation", false),
	)

	if proxyAddr != "" {
		opts = append(opts, chromedp.ProxyServer(proxyAddr))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	// 创建带超时的任务上下文 (含 Kiro OAuth 授权时上限 180 秒，纯 Google 登录上限 120 秒)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	timeoutDur := 120 * time.Second
	if autoAuthorizeKiro {
		timeoutDur = 180 * time.Second
	}
	ctx, cancelTimeout := context.WithTimeout(ctx, timeoutDur)
	defer cancelTimeout()

	// 注入底层反反爬脚本 (抹除 navigator.webdriver，伪装 window.chrome)
	stealthJS := `
(() => {
    Object.defineProperty(navigator, 'webdriver', {
        get: () => undefined
    });
    if (!window.chrome) {
        window.chrome = {};
    }
    window.chrome.runtime = window.chrome.runtime || {
        OnInstalledReason: { CHROME_UPDATE: 'chrome_update', INSTALL: 'install', UPDATE: 'update' },
        PlatformOs: { WIN: 'win', MAC: 'mac', LINUX: 'linux' }
    };
})();
`

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(c)
			return err
		}),
	); err != nil {
		log.Printf("[AutoLogin] 注入反检测脚本警告: %v", err)
	}

	startURL := "https://accounts.google.com/signin/v2/identifier?flowName=GlifWebSignIn&flowEntry=ServiceLogin&hl=en"
	report("正在打开 Google 登录页面...")

	if err := chromedp.Run(ctx, chromedp.Navigate(startURL)); err != nil {
		return fmt.Errorf("导航至登录页失败: %w", err)
	}

	// 登录状态机主循环
	lastState := ""
	emailEntered := false
	pwdEntered := false
	totpEntered := false
	recoveryEntered := false
	loggedIn := false

	report("开始识别登录界面状态...")

	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if loggedIn {
				break
			}
			return fmt.Errorf("自动化登录超时或被取消 (120s)")
		case <-ticker.C:
		}

		// 检查当前页面是否已成功登录
		var curURL string
		_ = chromedp.Run(ctx, chromedp.Location(&curURL))

		// 检查是否有登录成功的关键 Cookie 或已跳转到账号中心 / 授权页
		if isLoginSuccessURL(curURL) || checkHasLoginCookies(ctx) {
			report("检测到 Google 登录成功！")
			loggedIn = true
			break
		}

		// 巡检 DOM 状态
		var pageState string
		inspectJS := `
(() => {
    const isVisible = (el) => {
        if (!el) return false;
        const style = window.getComputedStyle(el);
        return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0' && el.offsetParent !== null;
    };

    // 1. 2FA 动态码输入框
    const totpEl = document.querySelector('#totpPin, input[name="totpPin"], input[type="tel"]');
    if (isVisible(totpEl)) return 'totp_input';

    // 2. 辅助邮箱直接输入框
    const recoveryInput = document.querySelector('input[name="knowledgePreregisteredEmailResponse"], input[id="knowledge-preregistered-email-response"]');
    if (isVisible(recoveryInput)) return 'recovery_input';

    // 3. 辅助挑战选项列表 (点击 "Confirm your recovery email")
    const challenges = Array.from(document.querySelectorAll('div[data-challengetype], li[data-challengetype], div[data-challengeindex], div[role="link"]'));
    for (const c of challenges) {
        const txt = (c.innerText || '').toLowerCase();
        const ctype = c.getAttribute('data-challengetype');
        if (ctype === '12' || txt.includes('recovery email') || txt.includes('辅助邮箱') || txt.includes('confirm your recovery')) {
            return 'recovery_challenge_option';
        }
    }

    // 4. 密码输入框
    const pwdEl = document.querySelector('input[type="password"]');
    if (isVisible(pwdEl)) return 'pwd_input';

    // 5. 账号输入框
    const emailEl = document.querySelector('input[type="email"], #identifierId');
    if (isVisible(emailEl)) return 'email_input';

    // 6. 安全提示跳过按钮 ("Not now" / "Skip")
    const btns = Array.from(document.querySelectorAll('button'));
    for (const b of btns) {
        const t = (b.innerText || '').trim().toLowerCase();
        if (t === 'not now' || t === '暂不' || t === '稍后' || t === 'skip' || t === '跳过') {
            return 'skip_btn';
        }
    }

    // 7. 错误提示检测
    const errEl = document.querySelector('div[aria-live="assertive"], .o6cuMc, .dEOOab');
    if (errEl && errEl.innerText && errEl.innerText.trim() !== '') {
        return 'error:' + errEl.innerText.trim();
    }

    return 'waiting';
})()
`
		if err := chromedp.Run(ctx, chromedp.Evaluate(inspectJS, &pageState)); err != nil {
			continue
		}

		if pageState != lastState {
			lastState = pageState
			log.Printf("[AutoLogin][%s] 页面状态转换: %s (URL: %s)", acc.Email, pageState, curURL)
		}

		switch {
		case pageState == "email_input" && !emailEntered:
			report("正在仿真输入 Google 账号...")
			emailSel := `input[type="email"], #identifierId`
			if err := simulateHumanType(ctx, emailSel, acc.Email); err != nil {
				log.Printf("[AutoLogin] 输入账号异常: %v", err)
				continue
			}
			time.Sleep(300 * time.Millisecond)
			report("正在提交账号...")
			_ = chromedp.Run(ctx, chromedp.Click(`#identifierNext button, #identifierNext`, chromedp.ByQuery))
			emailEntered = true
			time.Sleep(2 * time.Second)

		case pageState == "pwd_input" && !pwdEntered:
			report("正在仿真输入密码...")
			pwdSel := `input[type="password"]`
			if err := simulateHumanType(ctx, pwdSel, acc.Password); err != nil {
				log.Printf("[AutoLogin] 输入密码异常: %v", err)
				continue
			}
			time.Sleep(300 * time.Millisecond)
			report("正在提交密码...")
			_ = chromedp.Run(ctx, chromedp.Click(`#passwordNext button, #passwordNext`, chromedp.ByQuery))
			pwdEntered = true
			time.Sleep(2 * time.Second)

		case pageState == "totp_input" && !totpEntered:
			if strings.TrimSpace(acc.TOTPSecret) == "" {
				report("⚠️ 触发 2FA 验证但未配置 2FA 密钥，请在浏览器中手动输入！")
				time.Sleep(3 * time.Second)
				continue
			}
			report("检测到 2FA 验证，正在根据密钥计算当前 6 位动态验证码...")
			code, err := storage.GenerateTOTPCode(acc.TOTPSecret)
			if err != nil {
				report(fmt.Sprintf("计算 2FA 失败: %v", err))
				continue
			}
			report(fmt.Sprintf("正在填入 2FA 动态码 (%s)...", code))
			totpSel := `#totpPin, input[name="totpPin"], input[type="tel"]`
			if err := simulateHumanType(ctx, totpSel, code); err != nil {
				log.Printf("[AutoLogin] 输入 2FA 异常: %v", err)
				continue
			}
			time.Sleep(300 * time.Millisecond)
			report("正在提交 2FA 验证码...")
			_ = chromedp.Run(ctx, chromedp.Click(`#totpNext button, #totpNext`, chromedp.ByQuery))
			totpEntered = true
			time.Sleep(3 * time.Second)

		case pageState == "recovery_challenge_option":
			report("检测到辅助验证选项，正在选择‘确认辅助邮箱’...")
			clickRecoveryOptJS := `
(() => {
    const challenges = Array.from(document.querySelectorAll('div[data-challengetype], li[data-challengetype], div[data-challengeindex], div[role="link"]'));
    for (const c of challenges) {
        const txt = (c.innerText || '').toLowerCase();
        const ctype = c.getAttribute('data-challengetype');
        if (ctype === '12' || txt.includes('recovery email') || txt.includes('辅助邮箱') || txt.includes('confirm your recovery')) {
            c.click();
            return true;
        }
    }
    return false;
})()
`
			_ = chromedp.Run(ctx, chromedp.Evaluate(clickRecoveryOptJS, nil))
			time.Sleep(2 * time.Second)

		case pageState == "recovery_input" && !recoveryEntered:
			if strings.TrimSpace(acc.Recovery) == "" {
				report("⚠️ 触发辅助邮箱验证但未配置辅助邮箱，请在浏览器中手动输入！")
				time.Sleep(3 * time.Second)
				continue
			}
			report(fmt.Sprintf("正在输入辅助邮箱 (%s)...", acc.Recovery))
			recoverySel := `input[name="knowledgePreregisteredEmailResponse"], input[id="knowledge-preregistered-email-response"]`
			if err := simulateHumanType(ctx, recoverySel, acc.Recovery); err != nil {
				log.Printf("[AutoLogin] 输入辅助邮箱异常: %v", err)
				continue
			}
			time.Sleep(300 * time.Millisecond)
			report("正在提交辅助邮箱验证...")
			_ = chromedp.Run(ctx, chromedp.KeyEvent("\r"))
			clickNextJS := `
(() => {
    const btns = Array.from(document.querySelectorAll('button, div[role="button"]'));
    for (const b of btns) {
        const t = (b.innerText || '').trim().toLowerCase();
        if (t === 'next' || t === '下一步') {
            b.click();
            return true;
        }
    }
    return false;
})()
`
			_ = chromedp.Run(ctx, chromedp.Evaluate(clickNextJS, nil))
			recoveryEntered = true
			time.Sleep(3 * time.Second)

		case pageState == "skip_btn":
			report("检测到安全选项提示，正在点击跳过...")
			clickSkipJS := `
(() => {
    const btns = Array.from(document.querySelectorAll('button'));
    for (const b of btns) {
        const t = (b.innerText || '').trim().toLowerCase();
        if (t === 'not now' || t === '暂不' || t === '稍后' || t === 'skip' || t === '跳过') {
            b.click();
            return true;
        }
    }
    return false;
})()
`
			_ = chromedp.Run(ctx, chromedp.Evaluate(clickSkipJS, nil))
			time.Sleep(2 * time.Second)

		case strings.HasPrefix(pageState, "error:"):
			errMsg := strings.TrimPrefix(pageState, "error:")
			report(fmt.Sprintf("Google 提示错误: %s", errMsg))
			time.Sleep(2 * time.Second)
		}
	}

	// 标记状态为已登录
	_ = storage.UpdateGoogleAccountStatus(acc.Email, "logged_in", "已就绪 (本地会话有效)")

	// 若开启了自动流转至 Kiro 授权
	if autoAuthorizeKiro {
		report("正在自动流转至 Kiro 官方 Google OAuth 授权...")
		kiroURL := "https://app.kiro.dev/signin?login_provider=Google"
		if err := chromedp.Run(ctx, chromedp.Navigate(kiroURL)); err != nil {
			return fmt.Errorf("导航至 Kiro 登录页失败: %w", err)
		}

		report("正在等待 Google OAuth 授权页面响应...")
		time.Sleep(4 * time.Second)
		oauthSuccess := false
		oauthTicker := time.NewTicker(1200 * time.Millisecond)
		defer oauthTicker.Stop()

		oauthDeadline := time.Now().Add(90 * time.Second)
		lastOAuthURL := ""
		googleBtnClicked := false
		accountClicked := false
		consentClicked := false

		for time.Now().Before(oauthDeadline) {
			select {
			case <-ctx.Done():
				return fmt.Errorf("Kiro 授权超时或被取消")
			case <-oauthTicker.C:
			}

			var currentURL string
			_ = chromedp.Run(ctx, chromedp.Location(&currentURL))
			uLower := strings.ToLower(currentURL)

			if currentURL != lastOAuthURL {
				lastOAuthURL = currentURL
				report(fmt.Sprintf("OAuth 页面转换: %s", currentURL))
			}

			// 0. 网络波动自动重试
			if strings.Contains(uLower, "chrome-error") {
				report("检测到网络瞬时波动，正在重新加载授权页面...")
				googleBtnClicked = false
				accountClicked = false
				consentClicked = false
				_ = chromedp.Run(ctx, chromedp.Navigate(kiroURL))
				time.Sleep(3 * time.Second)
				continue
			}

			// 1. 成功判定：已进入 Kiro 内部页面 (非 signin)
			if strings.Contains(uLower, "app.kiro.dev") && !strings.Contains(uLower, "/signin") {
				report("🎉 成功完成 Kiro 注册与 Google OAuth 授权！")
				oauthSuccess = true
				break
			}

			// 2. 若在 Kiro signin 页面，先关闭 Cookie 弹窗并触发 Google 登录按钮
			if !googleBtnClicked && strings.Contains(uLower, "app.kiro.dev/signin") && !strings.Contains(uLower, "oauth") {
				clickGoogleBtnJS := `
(() => {
    // A. 如果有 AWS Cookie 弹窗，先点击 Accept 消除遮罩
    const acceptBtn = document.querySelector('.awsccc-u-btn-primary, button[id*="accept"]');
    if (acceptBtn && (acceptBtn.innerText || '').toLowerCase().includes('accept')) {
        acceptBtn.click();
    }

    // B. 点击 Google 登录按钮 (使用完整鼠标事件以穿透 React 虚拟事件捕获)
    const btns = Array.from(document.querySelectorAll('button, a, div[role="button"]'));
    for (const b of btns) {
        const t = (b.innerText || '').toLowerCase();
        const aria = (b.getAttribute('aria-label') || '').toLowerCase();
        if (t.includes('google') || aria.includes('google') || b.querySelector('img[src*="google"]')) {
            const rect = b.getBoundingClientRect();
            if (rect.width > 0 && rect.height > 0) {
                const opts = { bubbles: true, cancelable: true, view: window, clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2 };
                b.dispatchEvent(new MouseEvent('mousedown', opts));
                b.dispatchEvent(new MouseEvent('mouseup', opts));
                b.dispatchEvent(new MouseEvent('click', opts));
                return 'clicked_google_btn';
            }
            b.click();
            return 'clicked_google_btn';
        }
    }
    return '';
})()
`
				var action string
				_ = chromedp.Run(ctx, chromedp.Evaluate(clickGoogleBtnJS, &action))
				if action != "" {
					googleBtnClicked = true
					report("在 Kiro 登录页触发 Google 登录按钮，等待重定向至 Google 授权...")
					time.Sleep(5 * time.Second)
					continue
				}
			}

			// 3. Google OAuth: 账号选择页 (仅在 accountchooser URL 下执行选号)
			if strings.Contains(uLower, "accountchooser") {
				clickAccountJS := fmt.Sprintf(`
(() => {
    const targetEmail = %q.toLowerCase();
    const listItems = Array.from(document.querySelectorAll('li, div[role="link"], div[data-identifier], div[data-email]'));
    for (const item of listItems) {
        const txt = (item.innerText || item.getAttribute('data-identifier') || item.getAttribute('data-email') || '').toLowerCase();
        if (txt.includes(targetEmail)) {
            const clickable = item.querySelector('div[role="link"], div[data-identifier], div[data-email]') || item;
            const rect = clickable.getBoundingClientRect();
            if (rect.width > 0 && rect.height > 0) {
                const opts = { bubbles: true, cancelable: true, view: window, clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2 };
                clickable.dispatchEvent(new MouseEvent('mousedown', opts));
                clickable.dispatchEvent(new MouseEvent('mouseup', opts));
                clickable.dispatchEvent(new MouseEvent('click', opts));
                return 'clicked_account';
            }
            item.click();
            return 'clicked_account';
        }
    }
    return '';
})()
`, strings.ToLower(acc.Email))
				var action string
				_ = chromedp.Run(ctx, chromedp.Evaluate(clickAccountJS, &action))
				if action != "" && !accountClicked {
					accountClicked = true
					report("在账号选择列表中选定目标 Google 账号，等待跳转至授权页...")
					time.Sleep(3 * time.Second)
					continue
				}
			}

			// 4. Google OAuth: 授权确认页 (在 consent 或 signin/oauth/id 下执行确认)
			if (strings.Contains(uLower, "signin/oauth") || strings.Contains(uLower, "consent")) && !strings.Contains(uLower, "accountchooser") {
				clickConsentJS := `
(() => {
    // A. 先检查并点击“向下滚动”按钮 (เลื่อนลง / scroll down)
    const scrollBtns = Array.from(document.querySelectorAll('button')).filter(b => {
        const t = (b.innerText || '').toLowerCase();
        return t.includes('เลื่อนลง') || t.includes('scroll');
    });
    if (scrollBtns.length > 0) {
        scrollBtns[0].click();
        window.scrollTo(0, document.body.scrollHeight);
    }

    // B. OAuth 授权确认按钮 (支持英/泰/中/日等多国语言)
    const btns = Array.from(document.querySelectorAll('button, div[role="button"]'));
    const confirmWords = ['ดำเนินการต่อ', 'continue', 'allow', '继续', '允许', 'confirm', '确认', 'proceed', '次へ', '同意'];
    for (const b of btns) {
        const t = (b.innerText || '').trim().toLowerCase();
        for (const w of confirmWords) {
            if (t === w || t.includes(w)) {
                b.click();
                return 'clicked_consent: ' + t;
            }
        }
    }

    // C. 经典 Google 授权按钮 id
    const submitBtn = document.querySelector('#submit_approve_access');
    if (submitBtn) {
        submitBtn.click();
        return 'clicked_submit_approve';
    }

    return '';
})()
`
				var action string
				_ = chromedp.Run(ctx, chromedp.Evaluate(clickConsentJS, &action))
				if action != "" && !consentClicked {
					consentClicked = true
					report(fmt.Sprintf("检测到 Google 授权确认页，已点击‘允许 / 继续’ (%s)...", action))
					time.Sleep(4 * time.Second)
					continue
				}
			}
		}

		if oauthSuccess {
			_ = storage.UpdateGoogleAccountStatus(acc.Email, "authorized", "已完成 Kiro 注册与授权")
			CaptureKiroCookiesFromContext(ctx, acc.Email)
		} else {
			report("OAuth 授权等待超时，Google 已登录但未完成 Kiro 授权")
			_ = storage.UpdateGoogleAccountStatus(acc.Email, "logged_in", "Google 已登录，Kiro 授权等待超时")
		}
	} else {
		report("Google 账号登录完成，Cookies 已持久化！")
	}

	return nil
}

// simulateHumanType 仿真人类物理击键输入 (带随机延迟，规避 isTrusted 与按键频率检测)
func simulateHumanType(ctx context.Context, sel string, text string) error {
	// 等待元素可见并获得焦点
	if err := chromedp.Run(ctx,
		chromedp.WaitVisible(sel, chromedp.ByQuery),
		chromedp.Focus(sel, chromedp.ByQuery),
	); err != nil {
		return err
	}

	time.Sleep(time.Duration(100+rand.Intn(100)) * time.Millisecond)

	// 逐字符触发物理按键事件
	for _, ch := range text {
		s := string(ch)
		if err := chromedp.Run(ctx, chromedp.SendKeys(sel, s, chromedp.ByQuery)); err != nil {
			return err
		}
		// 50ms ~ 120ms 随机抖动间隔
		delay := time.Duration(50+rand.Intn(70)) * time.Millisecond
		time.Sleep(delay)
	}

	time.Sleep(time.Duration(150+rand.Intn(150)) * time.Millisecond)
	return nil
}

// isLoginSuccessURL 判断当前 URL 是否表明已登录成功
func isLoginSuccessURL(u string) bool {
	uLower := strings.ToLower(u)
	if strings.Contains(uLower, "myaccount.google.com") {
		return true
	}
	if strings.Contains(uLower, "mail.google.com") {
		return true
	}
	if strings.Contains(uLower, "accounts.google.com/b/") {
		return true
	}
	if strings.Contains(uLower, "app.kiro.dev") && (!strings.Contains(uLower, "/signin") || strings.Contains(uLower, "/signin/oauth")) {
		return true
	}
	return false
}

// checkHasLoginCookies 通过 CDP 检查是否已包含 Google 核心登录 Session Cookie
func checkHasLoginCookies(ctx context.Context) bool {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(c)
			return err
		}),
	)
	if err != nil {
		return false
	}

	hasSID := false
	hasSSID := false
	for _, ck := range cookies {
		if strings.Contains(ck.Domain, "google.com") {
			if ck.Name == "SID" {
				hasSID = true
			}
			if ck.Name == "SSID" {
				hasSSID = true
			}
		}
	}
	return hasSID && hasSSID
}
