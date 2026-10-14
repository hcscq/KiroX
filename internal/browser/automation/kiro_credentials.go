package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"reg_go/internal/storage"
)

// KiroTokenRefreshResponse Kiro 官方桌面 Refresh API 返回格式
type KiroTokenRefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ProfileArn   string `json:"profileArn"`
	ExpiresIn    int    `json:"expiresIn"`
}

// RefreshKiroToken 调用 Kiro 官方桌面 API 刷新 Token 并获取最新的 profileArn
func RefreshKiroToken(refreshToken string) (*KiroTokenRefreshResponse, error) {
	rtClean := strings.TrimSpace(refreshToken)
	if rtClean == "" {
		return nil, fmt.Errorf("refreshToken 为空")
	}

	reqBody, err := json.Marshal(map[string]string{
		"refreshToken": rtClean,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KiroIDE-0.6.18-desktop")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Kiro 刷新接口失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取刷新响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Kiro 刷新接口返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var result KiroTokenRefreshResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 Kiro 刷新响应失败: %w", err)
	}

	if result.RefreshToken == "" {
		result.RefreshToken = rtClean
	}
	if result.ExpiresIn <= 0 {
		result.ExpiresIn = 3600
	}

	return &result, nil
}

// ExtractKiroSessionFromProfile 启动指定 Google 账号的独立 Chrome Profile，从 app.kiro.dev 读取 Session Cookies 并刷新补全 ProfileArn
func ExtractKiroSessionFromProfile(acc storage.GoogleAccount, cb ProgressCallback) (*storage.GoogleAccount, error) {
	report := func(msg string) {
		log.Printf("[KiroCreds][%s] %s", acc.Email, msg)
		if cb != nil {
			cb(msg)
		}
	}

	report("正在检查并启动独立 Chrome Profile...")
	profileDir := acc.ProfileDir
	if profileDir == "" {
		profileDir = storage.GenGoogleProfilePath(acc.Email)
	}
	EnsureProfileUnlocked(profileDir)

	browserPath, err := FindBrowserPath()
	if err != nil {
		return nil, err
	}

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
		chromedp.Flag("headless", true), // 纯提取可无头执行
	)
	if proxyAddr != "" {
		opts = append(opts, chromedp.ProxyServer(chromeProxyServer(proxyAddr)))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(automationContext(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	enableProxyAuth(ctx, proxyAddr)

	ctx, cancelTimeout := context.WithTimeout(ctx, 45*time.Second)
	defer cancelTimeout()

	report("正在连接 Kiro Web 会话并校验登录态...")
	if err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate("https://app.kiro.dev/settings/account"),
		chromedp.Sleep(2500*time.Millisecond),
	); err != nil {
		return nil, fmt.Errorf("打开 Kiro 页面失败: %w", err)
	}

	report("正在读取 app.kiro.dev 凭据 Cookies...")
	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		var err error
		cookies, err = network.GetCookies().Do(c)
		return err
	})); err != nil {
		return nil, fmt.Errorf("读取 Cookies 失败: %w", err)
	}

	var rawRT, rawAT, rawUID string
	for _, ck := range cookies {
		if strings.Contains(ck.Domain, "kiro.dev") {
			switch ck.Name {
			case "RefreshToken":
				rawRT = ck.Value
			case "AccessToken":
				rawAT = ck.Value
			case "UserId":
				rawUID = ck.Value
			}
		}
	}

	if rawRT == "" {
		report("未检测到活跃会话，正在自动拉起 Google 单点登录完成授权...")
		_ = chromedp.Run(ctx,
			chromedp.Navigate("https://app.kiro.dev/signin?login_provider=Google"),
			chromedp.Sleep(5*time.Second),
			chromedp.ActionFunc(func(c context.Context) error {
				var err error
				cookies, err = network.GetCookies().Do(c)
				return err
			}),
		)
		for _, ck := range cookies {
			if strings.Contains(ck.Domain, "kiro.dev") {
				switch ck.Name {
				case "RefreshToken":
					rawRT = ck.Value
				case "AccessToken":
					rawAT = ck.Value
				case "UserId":
					rawUID = ck.Value
				}
			}
		}
	}

	if rawRT == "" {
		return nil, fmt.Errorf("未在当前 Profile 中找到 Kiro RefreshToken，请确认该 Google 账号在浏览器中处于正常登录状态")
	}

	report("成功读取到 RefreshToken，正在向 Kiro 官方同步最新 Token 与 ProfileArn...")
	refResp, err := RefreshKiroToken(rawRT)
	if err != nil {
		report(fmt.Sprintf("⚠️ 官方接口刷新失败 (%v)，回退使用 Cookie 中提取的数据", err))
		expTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		_ = storage.UpdateGoogleAccountKiroTokens(acc.Email, rawRT, rawAT, "", rawUID, expTime)
	} else {
		expTime := time.Now().Add(time.Duration(refResp.ExpiresIn) * time.Second).Format(time.RFC3339)
		finalAT := refResp.AccessToken
		if finalAT == "" {
			finalAT = rawAT
		}
		_ = storage.UpdateGoogleAccountKiroTokens(acc.Email, refResp.RefreshToken, finalAT, refResp.ProfileArn, rawUID, expTime)
		report("✅ Kiro 凭据已成功获取、验证并持久化！")
	}

	// 重新读取返回
	accounts := storage.GetGoogleAccounts()
	for _, a := range accounts {
		if strings.EqualFold(a.Email, acc.Email) {
			return &a, nil
		}
	}
	return &acc, nil
}

// CaptureKiroCookiesFromContext 在已打开 app.kiro.dev 的活跃 chromedp Context 中静默捕获 Cookies 并保存
func CaptureKiroCookiesFromContext(ctx context.Context, email string) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		var err error
		cookies, err = network.GetCookies().Do(c)
		return err
	}))
	if err != nil {
		return
	}

	var rawRT, rawAT, rawUID string
	for _, ck := range cookies {
		if strings.Contains(ck.Domain, "kiro.dev") {
			switch ck.Name {
			case "RefreshToken":
				rawRT = ck.Value
			case "AccessToken":
				rawAT = ck.Value
			case "UserId":
				rawUID = ck.Value
			}
		}
	}

	if rawRT != "" {
		go func() {
			refResp, err := RefreshKiroToken(rawRT)
			if err == nil && refResp != nil {
				expTime := time.Now().Add(time.Duration(refResp.ExpiresIn) * time.Second).Format(time.RFC3339)
				_ = storage.UpdateGoogleAccountKiroTokens(email, refResp.RefreshToken, refResp.AccessToken, refResp.ProfileArn, rawUID, expTime)
			} else {
				expTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
				_ = storage.UpdateGoogleAccountKiroTokens(email, rawRT, rawAT, "", rawUID, expTime)
			}
		}()
	}
}
