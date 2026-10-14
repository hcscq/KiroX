package storage

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	googleAccountsMu     sync.RWMutex
	googleAccountsCache  []GoogleAccount
	googleAccountsLoaded bool
)

func resetGoogleAccountsCache() {
	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()
	googleAccountsCache = nil
	googleAccountsLoaded = false
}

// GoogleAccount Google 账号定义
type GoogleAccount struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	Recovery         string `json:"recovery,omitempty"`         // 辅助邮箱
	TOTPSecret       string `json:"totpSecret,omitempty"`       // 2FA 密钥 (用于自动生成 TOTP)
	Proxy            string `json:"proxy,omitempty"`            // 绑定的独立代理 (如 http://user:pass@host:port)
	ProfileDir       string `json:"profileDir,omitempty"`       // 独立 user-data-dir 目录名
	Status           string `json:"status"`                     // uninitialized (未就绪) / logged_in (已就绪) / authorized (已授权) / error (异常)
	StatusMsg        string `json:"statusMsg,omitempty"`
	PlanType         string `json:"planType,omitempty"`         // 目标订阅套餐 (KIRO_PRO 等)
	PaymentURL       string `json:"paymentUrl,omitempty"`       // 生成的 Stripe Checkout URL
	PaymentCreatedAt string `json:"paymentCreatedAt,omitempty"` // 链接生成时间
	PaymentExpiresAt string `json:"paymentExpiresAt,omitempty"` // 链接失效时间预估 (生成+24h)
	PaymentStatus    string `json:"paymentStatus,omitempty"`    // ungenerated / generated / paid / failed
	BackupTOTPSecret string `json:"backupTotpSecret,omitempty"` // 上一次使用的旧 2FA 密钥 (安全防丢备份)
	TOTPUpdatedAt    string `json:"totpUpdatedAt,omitempty"`    // 2FA 最近更新时间
	KiroRefreshToken string `json:"kiroRefreshToken,omitempty"` // Kiro RefreshToken (长效刷新令牌 aor...)
	KiroAccessToken  string `json:"kiroAccessToken,omitempty"`  // Kiro AccessToken (访问令牌 aoa...)
	KiroProfileArn   string `json:"kiroProfileArn,omitempty"`   // Kiro profileArn (arn:aws:codewhisperer...)
	KiroUserId       string `json:"kiroUserId,omitempty"`       // Kiro userId
	KiroExpiresAt    string `json:"kiroExpiresAt,omitempty"`    // 凭据过期时间 (ISO / YYYY/MM/DD HH:mm:ss)
	KiroUpdatedAt    string `json:"kiroUpdatedAt,omitempty"`    // 凭据提取/更新时间
	AddedAt          string `json:"addedAt"`
	LastUsedAt       string `json:"lastUsedAt,omitempty"`
}

func getGoogleAccountsFilePath() string {
	return filepath.Join(GetDataDir(), "google_accounts.json")
}

// loadGoogleAccountsLocked 在已持有 googleAccountsMu 写锁的情况下读取账号列表（优先使用内存缓存）
func loadGoogleAccountsLocked() []GoogleAccount {
	if googleAccountsLoaded {
		out := make([]GoogleAccount, len(googleAccountsCache))
		copy(out, googleAccountsCache)
		return out
	}

	filePath := getGoogleAccountsFilePath()
	data, err := os.ReadFile(filePath)
	if err != nil {
		googleAccountsCache = []GoogleAccount{}
		googleAccountsLoaded = true
		return []GoogleAccount{}
	}

	var accounts []GoogleAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		log.Printf("[Storage] 解析 google_accounts.json 失败: %v", err)
		return []GoogleAccount{}
	}
	googleAccountsCache = make([]GoogleAccount, len(accounts))
	copy(googleAccountsCache, accounts)
	googleAccountsLoaded = true

	out := make([]GoogleAccount, len(accounts))
	copy(out, accounts)
	return out
}

// saveGoogleAccountsLocked 在已持有 googleAccountsMu 写锁的情况下原子写入磁盘并更新内存缓存
func saveGoogleAccountsLocked(accounts []GoogleAccount) error {
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 Google 账号失败: %w", err)
	}

	filePath := getGoogleAccountsFilePath()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filePath); err != nil {
		return err
	}

	googleAccountsCache = make([]GoogleAccount, len(accounts))
	copy(googleAccountsCache, accounts)
	googleAccountsLoaded = true
	return nil
}

// GetGoogleAccounts 读取所有 Google 账号
func GetGoogleAccounts() []GoogleAccount {
	googleAccountsMu.RLock()
	if googleAccountsLoaded {
		out := make([]GoogleAccount, len(googleAccountsCache))
		copy(out, googleAccountsCache)
		googleAccountsMu.RUnlock()
		return out
	}
	googleAccountsMu.RUnlock()

	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()
	return loadGoogleAccountsLocked()
}

// SaveGoogleAccounts 保存所有 Google 账号
func SaveGoogleAccounts(accounts []GoogleAccount) error {
	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()
	return saveGoogleAccountsLocked(accounts)
}

// GenGoogleProfilePath 为账号生成或获取独立 profile 绝对路径
func GenGoogleProfilePath(email string) string {
	emailClean := strings.ToLower(strings.TrimSpace(email))
	hash := sha256.Sum256([]byte(emailClean))
	folderName := fmt.Sprintf("profile_%s", hex.EncodeToString(hash[:8]))
	dir := filepath.Join(GetDataDir(), "google_profiles", folderName)
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// AddGoogleAccounts 批量添加 Google 账号
// 支持格式: 邮箱----密码----辅助邮箱----2FA密钥----代理URL (每行一个，后三项可选)
func AddGoogleAccounts(text string) map[string]interface{} {
	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()

	lines := strings.Split(text, "\n")
	existing := loadGoogleAccountsLocked()
	existingMap := make(map[string]int)
	for i, acc := range existing {
		existingMap[strings.ToLower(strings.TrimSpace(acc.Email))] = i
	}

	addedCount := 0
	updatedCount := 0
	now := time.Now().Format("2006-01-02 15:04:05")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "----")
		if len(parts) < 2 {
			// 尝试以竖线、逗号或制表符分割
			if strings.Contains(line, "|") {
				parts = strings.Split(line, "|")
			} else if strings.Contains(line, "\t") {
				parts = strings.Split(line, "\t")
			} else if strings.Contains(line, ",") {
				parts = strings.Split(line, ",")
			}
		}

		email := strings.TrimSpace(parts[0])
		if email == "" || !strings.Contains(email, "@") {
			continue
		}
		password := ""
		if len(parts) > 1 {
			password = strings.TrimSpace(parts[1])
		}
		recovery := ""
		if len(parts) > 2 {
			recovery = strings.TrimSpace(parts[2])
		}
		totpSecret := ""
		if len(parts) > 3 {
			totpSecret = strings.ReplaceAll(strings.TrimSpace(parts[3]), " ", "")
		}
		proxyURL := ""
		if len(parts) > 4 {
			proxyURL = strings.TrimSpace(parts[4])
		}

		key := strings.ToLower(email)
		profileDir := GenGoogleProfilePath(email)

		acc := GoogleAccount{
			Email:      email,
			Password:   password,
			Recovery:   recovery,
			TOTPSecret: totpSecret,
			Proxy:      proxyURL,
			ProfileDir: profileDir,
			Status:     "uninitialized",
			StatusMsg:  "未登录环境",
			AddedAt:    now,
		}

		if idx, found := existingMap[key]; found {
			prev := existing[idx]
			if acc.Password == "" {
				acc.Password = prev.Password
			}
			if acc.Recovery == "" {
				acc.Recovery = prev.Recovery
			}
			if acc.TOTPSecret == "" {
				acc.TOTPSecret = prev.TOTPSecret
			} else if prev.TOTPSecret != "" && !strings.EqualFold(prev.TOTPSecret, acc.TOTPSecret) {
				acc.BackupTOTPSecret = prev.TOTPSecret
				acc.TOTPUpdatedAt = now
			}
			if acc.Proxy == "" {
				acc.Proxy = prev.Proxy
			}
			if prev.ProfileDir != "" {
				acc.ProfileDir = prev.ProfileDir
			}
			// 保留原有状态、支付信息与已提取的 Kiro 凭据
			acc.Status = prev.Status
			acc.StatusMsg = prev.StatusMsg
			acc.PlanType = prev.PlanType
			acc.PaymentURL = prev.PaymentURL
			acc.PaymentCreatedAt = prev.PaymentCreatedAt
			acc.PaymentExpiresAt = prev.PaymentExpiresAt
			acc.PaymentStatus = prev.PaymentStatus
			if acc.BackupTOTPSecret == "" {
				acc.BackupTOTPSecret = prev.BackupTOTPSecret
			}
			if acc.TOTPUpdatedAt == "" {
				acc.TOTPUpdatedAt = prev.TOTPUpdatedAt
			}
			acc.KiroRefreshToken = prev.KiroRefreshToken
			acc.KiroAccessToken = prev.KiroAccessToken
			acc.KiroProfileArn = prev.KiroProfileArn
			acc.KiroUserId = prev.KiroUserId
			acc.KiroExpiresAt = prev.KiroExpiresAt
			acc.KiroUpdatedAt = prev.KiroUpdatedAt
			acc.AddedAt = prev.AddedAt
			acc.LastUsedAt = prev.LastUsedAt
			existing[idx] = acc
			updatedCount++
		} else {
			existing = append(existing, acc)
			existingMap[key] = len(existing) - 1
			addedCount++
		}
	}

	if err := saveGoogleAccountsLocked(existing); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	return map[string]interface{}{
		"success": true,
		"added":   addedCount,
		"updated": updatedCount,
		"total":   len(existing),
	}
}

// DeleteGoogleAccount 删除指定 Google 账号并清理其 profile
func DeleteGoogleAccount(email string) map[string]interface{} {
	emailClean := strings.ToLower(strings.TrimSpace(email))
	googleAccountsMu.Lock()
	existing := loadGoogleAccountsLocked()
	var newAccounts []GoogleAccount
	var profileToRemove string
	found := false

	for _, acc := range existing {
		if strings.ToLower(strings.TrimSpace(acc.Email)) == emailClean {
			found = true
			profileToRemove = acc.ProfileDir
			continue
		}
		newAccounts = append(newAccounts, acc)
	}

	if !found {
		googleAccountsMu.Unlock()
		return map[string]interface{}{"error": "账号不存在"}
	}

	if err := saveGoogleAccountsLocked(newAccounts); err != nil {
		googleAccountsMu.Unlock()
		return map[string]interface{}{"error": err.Error()}
	}
	googleAccountsMu.Unlock()

	if profileToRemove != "" {
		_ = os.RemoveAll(profileToRemove)
	}

	return map[string]interface{}{
		"success": true,
		"total":   len(newAccounts),
	}
}

// ClearGoogleAccounts 清空所有 Google 账号
func ClearGoogleAccounts() map[string]interface{} {
	googleAccountsMu.Lock()
	accounts := loadGoogleAccountsLocked()
	if err := saveGoogleAccountsLocked([]GoogleAccount{}); err != nil {
		googleAccountsMu.Unlock()
		return map[string]interface{}{"error": err.Error()}
	}
	googleAccountsMu.Unlock()

	for _, acc := range accounts {
		if acc.ProfileDir != "" {
			_ = os.RemoveAll(acc.ProfileDir)
		}
	}

	return map[string]interface{}{
		"success": true,
		"total":   0,
	}
}

// UpdateGoogleAccountStatus 更新账号运行状态
func UpdateGoogleAccountStatus(email, status, statusMsg string) error {
	emailClean := strings.ToLower(strings.TrimSpace(email))
	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()

	existing := loadGoogleAccountsLocked()
	found := false
	now := time.Now().Format("2006-01-02 15:04:05")

	for i := range existing {
		if strings.ToLower(strings.TrimSpace(existing[i].Email)) == emailClean {
			existing[i].Status = status
			existing[i].StatusMsg = statusMsg
			existing[i].LastUsedAt = now
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("未找到账号: %s", email)
	}
	return saveGoogleAccountsLocked(existing)
}

// UpdateGoogleAccountPayment 更新账号的订阅付款链接与状态
func UpdateGoogleAccountPayment(email, planType, paymentURL, expiresAt, status string) error {
	emailClean := strings.ToLower(strings.TrimSpace(email))
	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()

	existing := loadGoogleAccountsLocked()
	found := false
	now := time.Now().Format("2006-01-02 15:04:05")

	for i := range existing {
		if strings.ToLower(strings.TrimSpace(existing[i].Email)) == emailClean {
			if planType != "" {
				existing[i].PlanType = planType
			}
			if paymentURL != "" {
				existing[i].PaymentURL = paymentURL
				existing[i].PaymentCreatedAt = now
			}
			if expiresAt != "" {
				existing[i].PaymentExpiresAt = expiresAt
			}
			if status != "" {
				existing[i].PaymentStatus = status
			}
			existing[i].LastUsedAt = now
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("未找到账号: %s", email)
	}
	return saveGoogleAccountsLocked(existing)
}

// GenerateTOTPCode 根据 Base32 密钥计算当前 6 位 TOTP 动态验证码 (RFC 6238)
func GenerateTOTPCode(secret string) (string, error) {
	secretClean := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	if secretClean == "" {
		return "", fmt.Errorf("TOTP 密钥为空")
	}

	// 补齐 Base32 填充字符 '='
	missingPadding := len(secretClean) % 8
	if missingPadding > 0 {
		secretClean += strings.Repeat("=", 8-missingPadding)
	}

	key, err := base32.StdEncoding.DecodeString(secretClean)
	if err != nil {
		return "", fmt.Errorf("无效的 Base32 TOTP 密钥: %w", err)
	}

	// 计算当前 30 秒时间步长
	step := uint64(time.Now().Unix() / 30)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, step)

	// HMAC-SHA1
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	h := mac.Sum(nil)

	// 动态截断
	offset := h[len(h)-1] & 0x0f
	binaryCode := binary.BigEndian.Uint32(h[offset:offset+4]) & 0x7fffffff

	code := binaryCode % 1000000
	return fmt.Sprintf("%06d", code), nil
}

// UpdateGoogleAccount2FA 更新指定账号的 2FA 密钥，并将原密钥备份至 BackupTOTPSecret
func UpdateGoogleAccount2FA(email, newSecret string) error {
	emailClean := strings.ToLower(strings.TrimSpace(email))
	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()

	existing := loadGoogleAccountsLocked()
	found := false
	now := time.Now().Format("2006-01-02 15:04:05")

	newSecretClean := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(newSecret), " ", ""))

	for i := range existing {
		if strings.ToLower(strings.TrimSpace(existing[i].Email)) == emailClean {
			if existing[i].TOTPSecret != "" && existing[i].TOTPSecret != newSecretClean {
				existing[i].BackupTOTPSecret = existing[i].TOTPSecret
			}
			existing[i].TOTPSecret = newSecretClean
			existing[i].TOTPUpdatedAt = now
			existing[i].LastUsedAt = now
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("未找到账号: %s", email)
	}
	return saveGoogleAccountsLocked(existing)
}

// UpdateGoogleAccountCredentials 手动更新指定账号的凭据与配置 (密码、辅助邮箱、2FA密钥、独立代理)
// 注意：为避免仅保存凭据时清空 2FA 密钥，或仅保存 2FA 时清空辅助邮箱/代理，按实际传入字段安全更新。
func UpdateGoogleAccountCredentials(email, password, recovery, totpSecret, proxy string) error {
	emailClean := strings.ToLower(strings.TrimSpace(email))
	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()

	existing := loadGoogleAccountsLocked()
	found := false
	now := time.Now().Format("2006-01-02 15:04:05")

	totpClean := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(totpSecret), " ", ""))
	onlyUpdatingTOTP := password == "" && recovery == "" && proxy == "" && totpClean != ""

	for i := range existing {
		if strings.ToLower(strings.TrimSpace(existing[i].Email)) == emailClean {
			if onlyUpdatingTOTP {
				if existing[i].TOTPSecret != "" && existing[i].TOTPSecret != totpClean {
					existing[i].BackupTOTPSecret = existing[i].TOTPSecret
				}
				existing[i].TOTPSecret = totpClean
				existing[i].TOTPUpdatedAt = now
			} else {
				if password != "" {
					existing[i].Password = password
				}
				existing[i].Recovery = strings.TrimSpace(recovery)
				if totpClean != "" && existing[i].TOTPSecret != totpClean {
					if existing[i].TOTPSecret != "" {
						existing[i].BackupTOTPSecret = existing[i].TOTPSecret
					}
					existing[i].TOTPSecret = totpClean
					existing[i].TOTPUpdatedAt = now
				}
				existing[i].Proxy = strings.TrimSpace(proxy)
			}
			existing[i].LastUsedAt = now
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("未找到账号: %s", email)
	}
	return saveGoogleAccountsLocked(existing)
}

// UpdateGoogleAccountKiroTokens 更新指定账号的 Kiro 凭据 (RefreshToken, AccessToken, ProfileArn, UserId, ExpiresAt)
func UpdateGoogleAccountKiroTokens(email, rt, at, profileArn, userId, expiresAt string) error {
	emailClean := strings.ToLower(strings.TrimSpace(email))
	googleAccountsMu.Lock()
	defer googleAccountsMu.Unlock()

	existing := loadGoogleAccountsLocked()
	found := false
	now := time.Now().Format("2006-01-02 15:04:05")

	for i := range existing {
		if strings.ToLower(strings.TrimSpace(existing[i].Email)) == emailClean {
			if rt != "" {
				existing[i].KiroRefreshToken = strings.TrimSpace(rt)
			}
			if at != "" {
				existing[i].KiroAccessToken = strings.TrimSpace(at)
			}
			if profileArn != "" {
				existing[i].KiroProfileArn = strings.TrimSpace(profileArn)
			}
			if userId != "" {
				existing[i].KiroUserId = strings.TrimSpace(userId)
			}
			if expiresAt != "" {
				existing[i].KiroExpiresAt = strings.TrimSpace(expiresAt)
			}
			existing[i].KiroUpdatedAt = now
			existing[i].LastUsedAt = now
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("未找到账号: %s", email)
	}
	return saveGoogleAccountsLocked(existing)
}

func genUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func genMachineID(seed string) string {
	h := sha256.Sum256([]byte(seed + "_kirox_machine"))
	return hex.EncodeToString(h[:])
}

// BuildKiroCredential 将账号凭据转换为指定格式的结构体 (format: "kam" 或 "ide")
func BuildKiroCredential(acc GoogleAccount, format string) (interface{}, error) {
	if strings.TrimSpace(acc.KiroRefreshToken) == "" {
		return nil, fmt.Errorf("该账号尚未提取或生成 Kiro 凭据")
	}

	expDate := acc.KiroExpiresAt
	if expDate == "" {
		expDate = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	}

	if strings.ToLower(format) == "ide" {
		// Kiro IDE 官方格式 (kiro-auth-token.json)
		return map[string]interface{}{
			"accessToken":  acc.KiroAccessToken,
			"refreshToken": acc.KiroRefreshToken,
			"profileArn":   acc.KiroProfileArn,
			"expiresAt":    expDate,
			"authMethod":   "social",
			"provider":     "Google",
			"region":       "us-east-1",
		}, nil
	}

	// 默认: KAM 标准格式 (Kiro Account Manager / accounts.json 格式)
	kamAddedAt := acc.AddedAt
	if kamAddedAt == "" {
		kamAddedAt = time.Now().Format("2006/01/02 15:04:05")
	} else {
		kamAddedAt = strings.ReplaceAll(kamAddedAt, "-", "/")
	}

	kamExpiresAt := expDate
	// 如果是 RFC3339 时间格式，尝试转换为 YYYY/MM/DD HH:mm:ss
	if t, err := time.Parse(time.RFC3339, expDate); err == nil {
		kamExpiresAt = t.Format("2006/01/02 15:04:05")
	}

	planTitle := "FREE"
	if acc.PlanType != "" {
		planTitle = strings.ReplaceAll(acc.PlanType, "_", " ")
	}

	return map[string]interface{}{
		"id":           genUUID(),
		"email":        acc.Email,
		"password":     nil,
		"label":        acc.Email,
		"status":       "active",
		"addedAt":      kamAddedAt,
		"accessToken":  acc.KiroAccessToken,
		"refreshToken": acc.KiroRefreshToken,
		"expiresAt":    kamExpiresAt,
		"provider":     "Google",
		"userId":       acc.KiroUserId,
		"authMethod":   "social",
		"clientId":     nil,
		"clientSecret": nil,
		"region":       "us-east-1",
		"clientIdHash": nil,
		"ssoSessionId": nil,
		"idToken":      nil,
		"startUrl":     nil,
		"profileArn":   acc.KiroProfileArn,
		"usageData":    nil,
		"groupId":      nil,
		"tagLinks":     []string{},
		"machineId":    genMachineID(acc.Email),
		"subscription": map[string]interface{}{
			"type":  acc.PlanType,
			"title": planTitle,
		},
		"enabled": true,
	}, nil
}

// BuildKiroCredentialJSON 将单个账号凭据转换为 JSON 文本
func BuildKiroCredentialJSON(acc GoogleAccount, format string) (string, error) {
	cred, err := BuildKiroCredential(acc, format)
	if err != nil {
		return "", err
	}

	if strings.ToLower(format) == "kam" {
		// KAM 标准文件是顶级数组
		data, err := json.MarshalIndent([]interface{}{cred}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// BuildBatchKiroCredentialsJSON 将多个账号的 Kiro 凭据批量转换为 JSON 数组文本
func BuildBatchKiroCredentialsJSON(accs []GoogleAccount, format string) (string, error) {
	var list []interface{}
	for _, acc := range accs {
		if strings.TrimSpace(acc.KiroRefreshToken) == "" {
			continue
		}
		cred, err := BuildKiroCredential(acc, format)
		if err == nil {
			list = append(list, cred)
		}
	}

	if len(list) == 0 {
		return "", fmt.Errorf("选中的账号中均未包含有效的 Kiro 凭据")
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

