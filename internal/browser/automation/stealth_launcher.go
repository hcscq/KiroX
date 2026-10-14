package automation

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"reg_go/internal/storage"
)

// FindBrowserPath 探测本机可用的 Chrome / Edge / Chromium 路径
func FindBrowserPath() (string, error) {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		}
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	default: // linux
		candidates = []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
		}
	}

	for _, path := range candidates {
		if runtime.GOOS != "linux" {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		} else {
			if lp, err := exec.LookPath(path); err == nil {
				return lp, nil
			}
		}
	}

	return "", fmt.Errorf("未在系统中检测到 Google Chrome 或兼容 Chromium 浏览器，请安装 Google Chrome")
}

// BuildLaunchArgs 构造防指纹识别与环境隔离的启动参数
func BuildLaunchArgs(profileDir, proxyURL, startURL string) []string {
	args := []string{
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-blink-features=AutomationControlled",
		"--password-store=basic",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-features=CalculateNativeWinOcclusion,Translate,OptimizationHints,MediaRouter",
	}

	// 代理支持
	if proxyURL = strings.TrimSpace(proxyURL); proxyURL != "" {
		// Chrome --proxy-server 不支持 user:pass@，统一剥离账密 (交互式窗口中由 Chrome 弹框询问账密)
		args = append(args, "--proxy-server="+chromeProxyServer(proxyURL))
	}

	if startURL = strings.TrimSpace(startURL); startURL != "" {
		args = append(args, startURL)
	}

	return args
}

// LaunchInteractiveProfile 启动带独立 Profile 的浏览器供用户查看/登录环境
func LaunchInteractiveProfile(acc storage.GoogleAccount, targetURL string) error {
	browserPath, err := FindBrowserPath()
	if err != nil {
		return err
	}

	profileDir := acc.ProfileDir
	if profileDir == "" {
		profileDir = storage.GenGoogleProfilePath(acc.Email)
	}

	if targetURL == "" {
		targetURL = "https://accounts.google.com/"
	}

	proxyAddr := acc.Proxy
	if proxyAddr == "" {
		// 回退到全局代理
		proxyAddr = storage.GetProxy()
	}

	args := BuildLaunchArgs(profileDir, proxyAddr, targetURL)
	log.Printf("[Google Profile] 启动浏览器: %s, 账号: %s, 目录: %s", browserPath, acc.Email, profileDir)

	cmd := exec.Command(browserPath, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动浏览器进程失败: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()

	return nil
}

// LaunchKiroGoogleAuth 使用指定 Google 账号的独立 Profile 打开 Kiro 官方 Google OAuth 授权页面
// 访问 https://app.kiro.dev/signin?login_provider=Google 会自动触发 Kiro Web 前端向 Google 发起 OAuth 重定向
func LaunchKiroGoogleAuth(acc storage.GoogleAccount) error {
	kiroAuthURL := "https://app.kiro.dev/signin?login_provider=Google"
	return LaunchInteractiveProfile(acc, kiroAuthURL)
}

// EnsureProfileUnlocked 确保 Profile 目录未被现存 Chrome/Edge 进程占用，并清理陈旧锁文件
func EnsureProfileUnlocked(profileDir string) {
	if profileDir == "" {
		return
	}
	cleanDir := filepath.Clean(profileDir)

	lockFiles := []string{
		filepath.Join(cleanDir, "lockfile"),
		filepath.Join(cleanDir, "SingletonLock"),
		filepath.Join(cleanDir, "SingletonCookie"),
		filepath.Join(cleanDir, "SingletonSocket"),
	}

	stillLocked := false
	for _, lf := range lockFiles {
		if err := os.Remove(lf); err != nil && !os.IsNotExist(err) {
			stillLocked = true
		}
	}

	if !stillLocked {
		return
	}

	if runtime.GOOS == "windows" {
		escapedDir := strings.ReplaceAll(cleanDir, "'", "''")
		psScript := fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "name = 'chrome.exe' or name = 'msedge.exe'" | Where-Object { $_.CommandLine -and $_.CommandLine.IndexOf('%s', [System.StringComparison]::OrdinalIgnoreCase) -ge 0 } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`, escapedDir)
		psCmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", psScript)
		hideConsoleWindow(psCmd)
		_ = psCmd.Run()
		time.Sleep(300 * time.Millisecond)
	}

	for _, lf := range lockFiles {
		_ = os.Remove(lf)
	}
}


