package automation

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"reg_go/internal/storage"
)

// AutomateChangeGoogle2FA 自动化登录并修改 Google 账号的 2FA 身份验证器 (默认单窗口)
func AutomateChangeGoogle2FA(acc storage.GoogleAccount, cb ProgressCallback) (string, error) {
	return AutomateChangeGoogle2FAWithLayout(acc, cb, 0, 1)
}

// AutomateChangeGoogle2FAWithLayout 带有窗口排版支持的 Google 账号 2FA 自动化修改
func AutomateChangeGoogle2FAWithLayout(acc storage.GoogleAccount, cb ProgressCallback, workerIdx, concurrency int) (string, error) {
	report := func(msg string) {
		log.Printf("[Change2FA][%s] %s", acc.Email, msg)
		if cb != nil {
			cb(msg)
		}
	}

	winX, winY, winW, winH := CalculateWindowBounds(workerIdx, concurrency)
	report(fmt.Sprintf("正在启动独立 Chrome 环境 (窗口网格坐标: [%d, %d], 尺寸: %dx%d)...", winX, winY, winW, winH))

	browserPath, err := FindBrowserPath()
	if err != nil {
		return "", err
	}

	profileDir := acc.ProfileDir
	if profileDir == "" {
		profileDir = storage.GenGoogleProfilePath(acc.Email)
	}

	// 预先清理占用该 Profile 的残留 Chrome 进程与互斥锁文件，防止 ProcessSingleton 冲突导致崩溃
	EnsureProfileUnlocked(profileDir)

	proxyAddr := strings.TrimSpace(acc.Proxy)
	if proxyAddr == "" {
		proxyAddr = strings.TrimSpace(storage.GetProxy())
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserPath),
		chromedp.UserDataDir(profileDir),
		chromedp.Flag("window-position", fmt.Sprintf("%d,%d", winX, winY)),
		chromedp.Flag("window-size", fmt.Sprintf("%d,%d", winW, winH)),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
		chromedp.Flag("lang", "zh-CN,zh,en"),
		chromedp.Flag("headless", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-infobars", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"),
	)

	if proxyAddr != "" {
		report(fmt.Sprintf("正在挂载独立代理: %s", proxyAddr))
		opts = append(opts, chromedp.ProxyServer(chromeProxyServer(proxyAddr)))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(automationContext(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	enableProxyAuth(ctx, proxyAddr)

	// 设定总超时 4 分钟 (留足网络握手与身份核验时间)
	ctx, cancelTimeout := context.WithTimeout(ctx, 4*time.Minute)
	defer cancelTimeout()

	// 注入指纹反检测脚本
	stealthJS := `
(() => {
    Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
    window.chrome = { runtime: {} };
    const originalQuery = window.navigator.permissions.query;
    window.navigator.permissions.query = (parameters) => (
        parameters.name === 'notifications' ?
            Promise.resolve({ state: Notification.permission }) :
            originalQuery(parameters)
    );
})()
`
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(c)
			return err
		}),
	); err != nil {
		return "", fmt.Errorf("启动 Chrome 浏览器失败: %w", err)
	}

	targetURL := "https://myaccount.google.com/two-step-verification/authenticator?hl=en"
	report("正在导航至 Google 两步验证 (身份验证器) 设置页...")

	if err := chromedp.Run(ctx, chromedp.Navigate(targetURL)); err != nil {
		return "", fmt.Errorf("打开页面失败: %w", err)
	}

	emailEntered := false
	pwdEntered := false
	totpEntered := false
	recoveryEntered := false
	var newExtractedSecret string
	lastState := ""

	checkTicker := time.NewTicker(1500 * time.Millisecond)
	defer checkTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("修改 2FA 操作超时或被中断")
		case <-checkTicker.C:
		}

		var currentURL string
		_ = chromedp.Run(ctx, chromedp.Location(&currentURL))

		// 检查当前页面状态与元素 (必须利用 isVisible 过滤隐藏的上一阶段残留 DOM 元素)
		checkStateJS := `
(() => {
    const isVisible = (el) => {
        if (!el) return false;
        const style = window.getComputedStyle(el);
        return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0' && el.offsetParent !== null;
    };

    const url = window.location.href;
    const bodyText = (document.body ? document.body.innerText : '').toLowerCase();

    // 1. 弹窗状态检测 (验证器设置/替换弹窗)
    const dialog = document.querySelector('div[role="dialog"]');
    if (dialog && isVisible(dialog)) {
        const dText = (dialog.innerText || '').toLowerCase();

        // A. 若已有动态码输入框且处于输入验证阶段
        const verifyInput = dialog.querySelector('input[type="tel"], input[name="totpPin"]');
        if (verifyInput && isVisible(verifyInput)) {
            return 'dialog_verify_input';
        }

        // B. 优先检测是否已展示 32 位 Base32 密钥明文 (Google 官方格式: 32 位 Base32)
        const walker = document.createTreeWalker(dialog, NodeFilter.SHOW_TEXT, null, false);
        let n;
        while(n = walker.nextNode()) {
            const raw = (n.textContent || '').trim().replace(/\s+/g, '');
            if (raw.length === 32 && /^[2-7A-Za-z]{32}$/.test(raw)) {
                return 'dialog_key_extracted:' + raw.toUpperCase();
            }
            if ((raw.length === 16 || raw.length === 26) && /^[2-7A-Za-z]+$/.test(raw)) {
                const lower = raw.toLowerCase();
                if (!lower.includes('authenticator') && !lower.includes('google') && !lower.includes('setup')) {
                    return 'dialog_key_extracted:' + raw.toUpperCase();
                }
            }
        }

        // C. 若弹窗处于二维码展示阶段，检测到二维码图片或“无法扫描/Can't scan”按钮
        const normalizedDText = dText.replace(/[\u2018\u2019']/g, "'");
        const hasQRImg = dialog.querySelector('img[src^="data:image"], canvas') !== null;
        const hasCantScanBtn = dialog.querySelector('div[jsname="Ptcard"] button, div[jsname="Ptcard"]') !== null;
        if (hasQRImg || hasCantScanBtn || normalizedDText.includes("can't scan") || normalizedDText.includes("cant scan") || normalizedDText.includes("无法扫描") || normalizedDText.includes("scan it") || normalizedDText.includes("สแกนไม่ได้")) {
            return 'dialog_qr_showing';
        }

        if (dText.includes("verify") || dText.includes("验证") || dText.includes("enter the code") || dText.includes("enter code")) {
            return 'dialog_verify_input';
        }
    }

    // 2. 2FA 动态码核验框 (Sudo 鉴权)
    const totpEl = document.querySelector('#totpPin, input[name="totpPin"], input[type="tel"]');
    if (isVisible(totpEl)) {
        if (dialog && isVisible(dialog)) return 'dialog_verify_input';
        return 'totp_input';
    }

    // 3. 辅助邮箱挑战
    const recoveryInput = document.querySelector('input[name="knowledgePreregisteredEmailResponse"], input[id="knowledge-preregistered-email-response"]');
    if (isVisible(recoveryInput)) return 'recovery_input';

    const challenges = Array.from(document.querySelectorAll('div[data-challengetype], li[data-challengetype], div[data-challengeindex], div[role="link"]'));
    for (const c of challenges) {
        if (!isVisible(c)) continue;
        const txt = (c.innerText || '').toLowerCase();
        const ctype = c.getAttribute('data-challengetype');
        if (ctype === '12' || txt.includes('recovery email') || txt.includes('辅助邮箱') || txt.includes('confirm your recovery')) {
            return 'recovery_challenge_option';
        }
    }

    // 4. 密码输入框 (优先于隐藏的邮箱输入框判定)
    const pwdEl = document.querySelector('input[type="password"]');
    if (isVisible(pwdEl)) return 'pwd_input';

    // 5. 账号输入框
    const emailEl = document.querySelector('input[type="email"], #identifierId');
    if (isVisible(emailEl)) return 'email_input';

    // 6. 身份验证器列表页面
    if (url.includes('authenticator') || url.includes('two-step-verification') || url.includes('signinoptions')) {
        const btns = Array.from(document.querySelectorAll('button, div[role="button"], a[role="button"], a, span[role="button"]'));
        const btnTexts = [];
        for (const b of btns) {
            if (!isVisible(b)) continue;
            const t = (b.innerText || '').trim();
            const aria = (b.getAttribute('aria-label') || '').trim();
            if (t) btnTexts.push(t.replace(/\n+/g, ' '));
            if (t.includes('设置身份验证器') || t.includes('Set up authenticator') || t.includes('更改身份验证器') || t.includes('Change authenticator') || t.includes('设置') || t.includes('Set up') || t.includes('开始使用') || t.includes('Get started') || t.includes('开始') || t.includes('开启') || t.includes('Turn on') || t.includes('添加') || t.includes('Add') || aria.includes('身份验证器') || aria.includes('authenticator') || aria.includes('Set up') || aria.includes('Change')) {
                return 'btn_setup_authenticator_ready';
            }
        }
        return 'authenticator_page_loading: btns=[' + btnTexts.slice(0, 10).join(' | ') + '] body=' + (document.body ? document.body.innerText.substring(0, 150).replace(/\n+/g, ' ') : '');
    }

    return 'unknown';
})()
`
		var pageState string
		if err := chromedp.Run(ctx, chromedp.Evaluate(checkStateJS, &pageState)); err != nil {
			continue
		}

		if pageState != lastState {
			lastState = pageState
			log.Printf("[Change2FA][%s] 页面状态切换 -> %s (URL: %s)", acc.Email, pageState, currentURL)
		}

		// 分流状态逻辑
		switch {
		case pageState == "email_input" && !emailEntered:
			report("检测到账号输入界面，正在模拟填入邮箱...")
			emailSel := `input[type="email"], #identifierId`
			if err := simulateHumanType(ctx, emailSel, acc.Email); err != nil {
				continue
			}
			time.Sleep(300 * time.Millisecond)
			_ = chromedp.Run(ctx, chromedp.KeyEvent("\r"))
			_ = chromedp.Run(ctx, chromedp.Click(`#identifierNext button, #identifierNext`, chromedp.ByQuery))
			emailEntered = true
			time.Sleep(2 * time.Second)

		case pageState == "pwd_input" && !pwdEntered:
			report("正在核验账号密码...")
			pwdSel := `input[type="password"]`
			if err := simulateHumanType(ctx, pwdSel, acc.Password); err != nil {
				continue
			}
			time.Sleep(300 * time.Millisecond)
			_ = chromedp.Run(ctx, chromedp.KeyEvent("\r"))
			_ = chromedp.Run(ctx, chromedp.Click(`#passwordNext button, #passwordNext`, chromedp.ByQuery))
			pwdEntered = true
			time.Sleep(2 * time.Second)

		case pageState == "totp_input" && !totpEntered:
			report("检测到 Sudo 两步验证挑战，正在计算旧 2FA 动态码...")
			if strings.TrimSpace(acc.TOTPSecret) == "" {
				report("⚠️ 未提供旧 2FA 密钥，请在浏览器中手动完成核验！")
				time.Sleep(3 * time.Second)
				continue
			}
			code, err := storage.GenerateTOTPCode(acc.TOTPSecret)
			if err != nil {
				report(fmt.Sprintf("旧 2FA 计算失败: %v", err))
				continue
			}
			report(fmt.Sprintf("填入当前旧 2FA 动态码 (%s)...", code))
			totpSel := `#totpPin, input[name="totpPin"], input[type="tel"]`
			_ = simulateHumanType(ctx, totpSel, code)
			time.Sleep(300 * time.Millisecond)
			_ = chromedp.Run(ctx, chromedp.KeyEvent("\r"))
			_ = chromedp.Run(ctx, chromedp.Click(`#totpNext button, #totpNext`, chromedp.ByQuery))
			totpEntered = true
			time.Sleep(3 * time.Second)

		case pageState == "recovery_challenge_option":
			report("检测到辅助验证选项，正在选择‘确认辅助邮箱’...")
			clickRecoveryJS := `
(() => {
    const challenges = Array.from(document.querySelectorAll('div[data-challengetype], li[data-challengetype], div[data-challengeindex], div[role="link"], li'));
    for (const c of challenges) {
        const txt = (c.innerText || '').toLowerCase();
        const ctype = c.getAttribute('data-challengetype');
        if (ctype === '12' || txt.includes('recovery email') || txt.includes('辅助邮箱') || txt.includes('confirm your recovery') || txt.includes('确认您的辅助邮箱')) {
            c.scrollIntoView();
            c.click();
            const inner = c.querySelector('button, [role="button"], div');
            if (inner) inner.click();
            return true;
        }
    }
    return false;
})()
`
			var clicked bool
			_ = chromedp.Run(ctx, chromedp.Evaluate(clickRecoveryJS, &clicked))
			time.Sleep(2500 * time.Millisecond)

		case pageState == "recovery_input" && !recoveryEntered:
			if strings.TrimSpace(acc.Recovery) == "" {
				report("⚠️ 触发辅助邮箱验证但账号未配置辅助邮箱，请在浏览器窗口手动输入！")
				time.Sleep(3 * time.Second)
				continue
			}
			report(fmt.Sprintf("正在输入辅助邮箱 (%s)...", acc.Recovery))
			recoverySel := `input[name="knowledgePreregisteredEmailResponse"], input[id="knowledge-preregistered-email-response"], input[type="email"]`
			_ = simulateHumanType(ctx, recoverySel, acc.Recovery)
			time.Sleep(300 * time.Millisecond)
			_ = chromedp.Run(ctx, chromedp.KeyEvent("\r"))
			clickNextJS := `
(() => {
    const btns = Array.from(document.querySelectorAll('button, div[role="button"]'));
    for (const b of btns) {
        const txt = (b.innerText || '').trim();
        if (txt === '下一步' || txt === 'Next' || txt === '确认' || txt === 'Confirm') {
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

		case pageState == "btn_setup_authenticator_ready":
			report("已进入身份验证器页面，正在点击‘设置/更改身份验证器’...")
			clickSetupJS := `
(() => {
    const btns = Array.from(document.querySelectorAll('button, div[role="button"], a[role="button"], a'));
    for (const b of btns) {
        const t = (b.innerText || '').trim();
        if (t.includes('设置身份验证器') || t.includes('Set up authenticator') || t.includes('更改身份验证器') || t.includes('Change authenticator') || t.includes('设置') || t.includes('Set up') || t.includes('开始使用') || t.includes('Get started') || t.includes('开始')) {
            b.scrollIntoView();
            b.click();
            return true;
        }
    }
    return false;
})()
`
			_ = chromedp.Run(ctx, chromedp.Evaluate(clickSetupJS, nil))
			time.Sleep(2 * time.Second)

		case pageState == "dialog_qr_showing":
			report("正在点击‘无法扫描二维码？’以提取密钥明文...")
			clickCantScanJS := `
(() => {
    const dialog = document.querySelector('div[role="dialog"]');
    if (!dialog) return false;

    // 1. Google 官方专用特征选择器 (jsname="Ptcard")
    const ptcard = dialog.querySelector('div[jsname="Ptcard"] button, div[jsname="Ptcard"]');
    if (ptcard) {
        ptcard.click();
        return true;
    }

    // 2. 遍历所有可点击的按钮与链接 (严格排除 span, p 等非交互提示文本)
    const clickable = Array.from(dialog.querySelectorAll('button, div[role="button"], a'));
    for (const el of clickable) {
        const t = (el.innerText || '').trim().toLowerCase().replace(/[\u2018\u2019']/g, "'");
        if (t.includes("can't scan") || t.includes("cant scan") || t.includes("scan it") || t.includes("无法扫描") || t.includes("สแกนไม่ได้")) {
            el.click();
            return true;
        }
    }

    // 3. 兜底策略：查找包含此类文本且带指针样式的任意容器
    const all = Array.from(dialog.querySelectorAll('*'));
    for (const el of all) {
        const t = (el.innerText || '').trim().toLowerCase().replace(/[\u2018\u2019']/g, "'");
        if (t === "can't scan it?" || t === "can't scan it" || t === "无法扫描？" || t === "无法扫描二维码？") {
            el.click();
            return true;
        }
    }
    return false;
})()
`
			_ = chromedp.Run(ctx, chromedp.Evaluate(clickCantScanJS, nil))
			time.Sleep(1500 * time.Millisecond)

		case strings.HasPrefix(pageState, "dialog_key_extracted:"):
			rawKey := strings.TrimPrefix(pageState, "dialog_key_extracted:")
			if newExtractedSecret == "" && rawKey != "" {
				newExtractedSecret = rawKey
				report(fmt.Sprintf("🎉 成功提取到新 2FA 密钥: %s", newExtractedSecret))
				report("正在点击‘下一步’进行验证码激活核验...")

				clickNextDialogJS := `
(() => {
    const dialog = document.querySelector('div[role="dialog"]');
    if (!dialog) return false;
    const btns = Array.from(dialog.querySelectorAll('button, div[role="button"]'));
    for (const b of btns) {
        const t = (b.innerText || '').trim().toLowerCase();
        if (t === 'next' || t === '下一步' || t === 'continue' || t === '继续' || t.includes('next') || t.includes('下一步') || t.includes('ถัดไป')) {
            b.click();
            return true;
        }
    }
    return false;
})()
`
				_ = chromedp.Run(ctx, chromedp.Evaluate(clickNextDialogJS, nil))
				time.Sleep(2 * time.Second)
			}

		case pageState == "dialog_verify_input" && newExtractedSecret != "":
			report("检测到新验证码输入框，正在根据提取的新密钥计算动态码...")
			newCode, err := storage.GenerateTOTPCode(newExtractedSecret)
			if err != nil {
				return "", fmt.Errorf("计算新 TOTP 码失败: %w", err)
			}
			report(fmt.Sprintf("正在仿真输入激活动态码 (%s)...", newCode))

			fillNewCodeJS := fmt.Sprintf(`
(() => {
    const dialog = document.querySelector('div[role="dialog"]');
    if (!dialog) return false;
    const input = dialog.querySelector('input[type="tel"], input[type="text"], input[name="totpPin"]');
    if (!input) return false;
    input.focus();
    input.value = '%s';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
    return true;
})()
`, newCode)
			_ = chromedp.Run(ctx, chromedp.Evaluate(fillNewCodeJS, nil))
			time.Sleep(500 * time.Millisecond)

			report("正在提交验证以激活新 2FA...")
			clickVerifyDialogJS := `
(() => {
    const dialog = document.querySelector('div[role="dialog"]');
    if (!dialog) return false;
    const btns = Array.from(dialog.querySelectorAll('button, div[role="button"]'));
    for (const b of btns) {
        const t = (b.innerText || '').trim().toLowerCase();
        if (t === 'verify' || t === 'done' || t === 'save' || t === '验证' || t === '完成' || t === '保存' || t.includes('verify') || t.includes('done') || t.includes('ยืนยัน') || t.includes('เสร็จ')) {
            b.click();
            return true;
        }
    }
    return false;
})()
`
			_ = chromedp.Run(ctx, chromedp.Evaluate(clickVerifyDialogJS, nil))
			time.Sleep(3 * time.Second)

			// 检查是否成功完成
			checkDoneJS := `
(() => {
    const dialog = document.querySelector('div[role="dialog"]');
    if (!dialog || window.getComputedStyle(dialog).display === 'none') return true; // 对话框已关闭说明已成功
    const text = (dialog.innerText || '').toLowerCase();
    if (text.includes('完成') || text.includes('done') || text.includes('成功') || text.includes('set up') || text.includes('เสร็จ')) {
        const doneBtn = Array.from(dialog.querySelectorAll('button')).find(b => {
            const t = (b.innerText || '').toLowerCase();
            return t.includes('完成') || t.includes('done') || t.includes('เสร็จ');
        });
        if (doneBtn) doneBtn.click();
        return true;
    }
    return false;
})()
`
			var isDone bool
			_ = chromedp.Run(ctx, chromedp.Evaluate(checkDoneJS, &isDone))
			if isDone {
				report("✅ Google 账号 2FA 密钥已成功在谷歌安全中心重置并激活！")
				// 原子回写存储
				if err := storage.UpdateGoogleAccount2FA(acc.Email, newExtractedSecret); err != nil {
					log.Printf("[Change2FA] 回写存储失败: %v", err)
				}
				_ = storage.UpdateGoogleAccountStatus(acc.Email, "authorized", fmt.Sprintf("2FA已于 %s 成功重置", time.Now().Format("01-02 15:04")))
				return newExtractedSecret, nil
			}
		}
	}
}
