package automation

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
)

// chromeProxyServer 将代理 URL 规范化为 Chrome --proxy-server 可接受的格式。
// Chromium 命令行不支持 user:pass@ 形式的认证信息，这里统一剥离账密，
// 账密由 enableProxyAuth 通过 CDP Fetch.authRequired 应答。
func chromeProxyServer(proxyURL string) string {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return ""
	}
	if !strings.Contains(proxyURL, "://") {
		proxyURL = "http://" + proxyURL
	}
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return proxyURL
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, u.Host)
}

// proxyCredentials 提取代理 URL 中的账密 (无账密时 ok=false)
func proxyCredentials(proxyURL string) (user, pass string, ok bool) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return "", "", false
	}
	if !strings.Contains(proxyURL, "://") {
		proxyURL = "http://" + proxyURL
	}
	u, err := url.Parse(proxyURL)
	if err != nil || u.User == nil || u.User.Username() == "" {
		return "", "", false
	}
	p, _ := u.User.Password()
	return u.User.Username(), p, true
}

// enableProxyAuth 当代理带账密时，开启 CDP Fetch 认证拦截并自动应答代理认证。
// 注意: 仅作用于当前 target (主标签页)；弹出的新窗口不会自动带上认证。
// 对 SOCKS5 带账密代理 Chrome 本身不支持，需改用 HTTP 代理或本地转发。
func enableProxyAuth(ctx context.Context, proxyURL string) {
	user, pass, ok := proxyCredentials(proxyURL)
	if !ok {
		return
	}
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			go func() {
				resp := &fetch.AuthChallengeResponse{Response: fetch.AuthChallengeResponseResponseCancelAuth}
				if e.AuthChallenge != nil && e.AuthChallenge.Source == fetch.AuthChallengeSourceProxy {
					resp = &fetch.AuthChallengeResponse{
						Response: fetch.AuthChallengeResponseResponseProvideCredentials,
						Username: user,
						Password: pass,
					}
				}
				_ = chromedp.Run(ctx, fetch.ContinueWithAuth(e.RequestID, resp))
			}()
		case *fetch.EventRequestPaused:
			go func() { _ = chromedp.Run(ctx, fetch.ContinueRequest(e.RequestID)) }()
		}
	})
	if err := chromedp.Run(ctx, fetch.Enable().WithHandleAuthRequests(true)); err != nil {
		log.Printf("[Proxy] 启用代理认证拦截失败: %v", err)
	}
}
