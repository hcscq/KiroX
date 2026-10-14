package automation

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"reg_go/internal/storage"
)

// CheckProfileHasGoogleCookies 检查 Profile 目录下是否已生成 Google 登录凭据 Cookie 文件
func CheckProfileHasGoogleCookies(profileDir string) bool {
	if profileDir == "" {
		return false
	}

	cookiePaths := []string{
		filepath.Join(profileDir, "Default", "Network", "Cookies"),
		filepath.Join(profileDir, "Default", "Cookies"),
		filepath.Join(profileDir, "Network", "Cookies"),
		filepath.Join(profileDir, "Cookies"),
	}

	for _, p := range cookiePaths {
		if fi, err := os.Stat(p); err == nil && fi.Size() > 1024 {
			// 文件大小超过 1KB 说明已有 Cookie 写入
			return true
		}
	}
	return false
}

// SyncAccountLoginStatus 探测并同步指定 Google 账号的 Profile 状态
func SyncAccountLoginStatus(email string) string {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	emailClean := strings.ToLower(strings.TrimSpace(email))

	for i := range accounts {
		if strings.ToLower(strings.TrimSpace(accounts[i].Email)) == emailClean {
			target = &accounts[i]
			break
		}
	}

	if target == nil {
		return "unknown"
	}

	hasCookies := CheckProfileHasGoogleCookies(target.ProfileDir)
	status := "uninitialized"
	statusMsg := "未登录环境"

	if hasCookies {
		if target.Status == "authorized" {
			status = "authorized"
			statusMsg = "已授权 (Kiro Web 已就绪)"
		} else {
			status = "logged_in"
			statusMsg = "已就绪 (本地会话有效)"
		}
	}

	_ = storage.UpdateGoogleAccountStatus(target.Email, status, statusMsg)
	return status
}

// SyncAllGoogleAccountsStatus 批量同步所有账号的本地环境状态
func SyncAllGoogleAccountsStatus() {
	accounts := storage.GetGoogleAccounts()
	for _, acc := range accounts {
		SyncAccountLoginStatus(acc.Email)
	}
	log.Printf("[Google Automation] 已同步 %d 个账号的本地 Profile 状态", len(accounts))
}
