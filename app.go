package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"reg_go/internal/browser"
	"reg_go/internal/browser/automation"
	"reg_go/internal/email"
	"reg_go/internal/proxy"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reg_go/internal/storage"
	"reg_go/internal/task"
	"reg_go/internal/updater"
	"strings"
	"sync"
	"time"
)

type App struct {
	ctx                context.Context
	startupDone        chan struct{}
	runningGoogleTasks sync.Map // map[string]string: lowercase email -> task description
}

// NewApp 创建新的 App 实例
func NewApp() *App {
	return &App{startupDone: make(chan struct{})}
}

// lockGoogleAccount 尝试为指定 Google 账号加互斥锁，返回释放函数与可能的冲突错误
func (a *App) lockGoogleAccount(email, taskDesc string) (func(), error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	if normEmail == "" {
		return func() {}, nil
	}
	existing, loaded := a.runningGoogleTasks.LoadOrStore(normEmail, taskDesc)
	if loaded {
		return nil, fmt.Errorf("账号 [%s] 正在执行任务 (%v)，请稍候或勿重复点击", email, existing)
	}
	return func() {
		a.runningGoogleTasks.Delete(normEmail)
	}, nil
}

// IsGoogleAccountBusy 检查账号是否正在执行后台自动化任务
func (a *App) IsGoogleAccountBusy(email string) bool {
	normEmail := strings.ToLower(strings.TrimSpace(email))
	_, ok := a.runningGoogleTasks.Load(normEmail)
	return ok
}

// GetRunningGoogleTasks 返回当前所有正在执行自动化任务的账号及任务名称
func (a *App) GetRunningGoogleTasks() map[string]string {
	result := make(map[string]string)
	a.runningGoogleTasks.Range(func(key, value interface{}) bool {
		if k, ok := key.(string); ok {
			if v, ok := value.(string); ok {
				result[k] = v
			}
		}
		return true
	})
	return result
}

// startup 在应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	close(a.startupDone)
	// 重定向日志到内存
	log.SetOutput(&logWriter{app: a})
	log.SetFlags(log.Ltime)

	// 初始化代理池（按数据目录持久化）
	proxy.InitPool(storage.GetDataDir())
	settings := storage.GetAppSettings()
	storage.ConfigurePersistentLogs(settings.PersistentLogs, settings.LogRetentionDays)

	// 居中并显示窗口
	go func() {
		time.Sleep(200 * time.Millisecond)
		runtime.WindowCenter(ctx)
		runtime.WindowUnminimise(ctx)
		runtime.WindowShow(ctx)
	}()

}

// restoreWindow brings the existing window back when the executable is launched again.
func (a *App) restoreWindow() {
	<-a.startupDone
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}

// shutdown 在应用关闭时调用
func (a *App) shutdown(ctx context.Context) {
	storage.FlushAccountsSync()
}

// OpenURL 在系统默认浏览器中打开 URL
func (a *App) OpenURL(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

// ShowDesktopNotification displays a native desktop notification when supported.
func (a *App) ShowDesktopNotification(title, message string) map[string]interface{} {
	if err := showDesktopNotification(title, message); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true}
}

// logWriter 自定义日志写入器，根据运行状态路由日志
type logWriter struct {
	app *App
}

func (w *logWriter) Write(p []byte) (int, error) {
	msg := string(p)
	task.Manager.AppendLog(msg)
	storage.AppendPersistentLog(msg)
	return os.Stderr.Write(p)
}

// GetStatus 获取任务状态
func (a *App) GetStatus() map[string]interface{} {
	return task.Manager.GetStatus()
}

// GetLogs 获取日志
func (a *App) GetLogs() []string {
	return task.Manager.GetLogs()
}

// GetOverview 获取全局概览数据
func (a *App) GetOverview() map[string]interface{} {
	// Outlook 账号统计
	outlookTotal, outlookRegistered, outlookSuccess, outlookPending := countOutlookAccounts()

	// 当前任务状态
	taskStatus := task.Manager.GetStatus()

	return map[string]interface{}{
		"version": updater.GetCurrentVersion(),
		"kiro": map[string]interface{}{
			"taskRunning":   taskStatus["running"],
			"taskSuccess":   taskStatus["success"],
			"taskFailed":    taskStatus["failed"],
			"taskCompleted": taskStatus["completed"],
			"taskTotal":     taskStatus["total"],
		},
		"outlook": map[string]interface{}{
			"total":      outlookTotal,
			"registered": outlookRegistered,
			"success":    outlookSuccess,
			"pending":    outlookPending,
		},
	}
}

// GetTaskStatus 获取实时任务状态
func (a *App) GetTaskStatus() map[string]interface{} {
	taskStatus := task.Manager.GetStatus()
	return map[string]interface{}{
		"kiro": map[string]interface{}{
			"taskRunning":   taskStatus["running"],
			"taskSuccess":   taskStatus["success"],
			"taskFailed":    taskStatus["failed"],
			"taskCompleted": taskStatus["completed"],
			"taskTotal":     taskStatus["total"],
		},
	}
}

// countOutlookAccounts 统计 Outlook 账号
func countOutlookAccounts() (total, registered, success, pending int) {
	accounts := storage.GetAccountsCached()
	if len(accounts) == 0 {
		return
	}
	total = len(accounts)
	for _, acc := range accounts {
		reg, _ := acc["registered"].(bool)
		suc, _ := acc["success"].(bool)
		if reg {
			registered++
			if suc {
				success++
			}
		} else {
			pending++
		}
	}
	return
}

// ---- MoeMail ----

func (a *App) GetMoeMailConfigs() []email.MoeMailConfig {
	return email.GetMoeMailConfigs()
}

func (a *App) SaveMoeMailConfigs(configsJSON string) map[string]interface{} {
	return email.SaveMoeMailConfigs(configsJSON)
}

func (a *App) TestMoeMailConnection(configJSON string) map[string]interface{} {
	return email.TestMoeMailConnection(configJSON)
}

// ---- CloudMail ----

func (a *App) GetCloudMailConfigs() []email.CloudMailConfig {
	return email.GetCloudMailConfigs()
}

func (a *App) SaveCloudMailConfigs(configsJSON string) map[string]interface{} {
	return email.SaveCloudMailConfigs(configsJSON)
}

func (a *App) TestCloudMailConnection(configJSON string) map[string]interface{} {
	return email.TestCloudMailConnection(configJSON)
}

// ---- Outlook ----

func (a *App) AddOutlookAccounts(data string) map[string]interface{} {
	return email.AddOutlookAccounts(data)
}

func (a *App) GetOutlookAccounts() []map[string]interface{} {
	return email.GetOutlookAccounts()
}

func (a *App) DeleteOutlookAccount(em string) map[string]interface{} {
	return email.DeleteOutlookAccount(em)
}

func (a *App) ClearOutlookAccounts() map[string]interface{} {
	return email.ClearOutlookAccounts()
}

func (a *App) ClearRegisteredOutlookAccounts() map[string]interface{} {
	return email.ClearRegisteredOutlookAccounts()
}

func (a *App) ImportOutlookFile(filePath string) map[string]interface{} {
	return email.ImportOutlookFile(filePath)
}

// ---- iCloud ----

func (a *App) AddICloudAccounts(data string) map[string]interface{} {
	return email.AddICloudAccounts(data)
}

func (a *App) GetICloudAccounts() []map[string]interface{} {
	return email.GetICloudAccounts()
}

func (a *App) DeleteICloudAccount(em string) map[string]interface{} {
	return email.DeleteICloudAccount(em)
}

func (a *App) ClearICloudAccounts() map[string]interface{} {
	return email.ClearICloudAccounts()
}

func (a *App) ImportICloudFile(filePath string) map[string]interface{} {
	return email.ImportICloudFile(filePath)
}

// ---- MailNest ----

func (a *App) TestMailNestConnection(configJSON string) map[string]interface{} {
	return email.TestMailNestConnection(configJSON)
}

func (a *App) SaveMailNestConfig(configsJSON string) map[string]interface{} {
	return email.SaveMailNestConfig(configsJSON)
}

func (a *App) GetMailNestConfig() email.MailNestConfig {
	return email.GetMailNestConfig()
}

// ---- Google Accounts ----

// GetGoogleAccounts 获取 Google 账号列表
func (a *App) GetGoogleAccounts() []storage.GoogleAccount {
	return storage.GetGoogleAccounts()
}

// AddGoogleAccounts 批量添加 Google 账号
func (a *App) AddGoogleAccounts(data string) map[string]interface{} {
	return storage.AddGoogleAccounts(data)
}

// DeleteGoogleAccount 删除单个 Google 账号
func (a *App) DeleteGoogleAccount(em string) map[string]interface{} {
	return storage.DeleteGoogleAccount(em)
}

// ClearGoogleAccounts 清空所有 Google 账号
func (a *App) ClearGoogleAccounts() map[string]interface{} {
	return storage.ClearGoogleAccounts()
}

// OpenGoogleProfile 拉起独立 Profile 浏览器窗口供用户操作/登录
func (a *App) OpenGoogleProfile(em string) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	for i := range accounts {
		if accounts[i].Email == em {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return map[string]interface{}{"error": "未找到指定账号"}
	}

	if a.IsGoogleAccountBusy(em) {
		return map[string]interface{}{"error": fmt.Sprintf("账号 [%s] 正在执行后台自动化任务，请稍后再操作环境", em)}
	}

	if err := automation.LaunchInteractiveProfile(*target, "https://accounts.google.com/"); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	// 异步更新并同步一次状态
	go func() {
		time.Sleep(3 * time.Second)
		automation.SyncAccountLoginStatus(em)
	}()

	return map[string]interface{}{"success": true}
}

// AuthorizeKiroWithGoogle 拉起指定 Google 账号独立 Profile 并跳转 Kiro 官方 Google OAuth 授权
func (a *App) AuthorizeKiroWithGoogle(em string) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	for i := range accounts {
		if accounts[i].Email == em {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return map[string]interface{}{"error": "未找到指定账号"}
	}

	if a.IsGoogleAccountBusy(em) {
		return map[string]interface{}{"error": fmt.Sprintf("账号 [%s] 正在执行后台自动化任务，请稍后再拉起授权", em)}
	}

	if err := automation.LaunchKiroGoogleAuth(*target); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	_ = storage.UpdateGoogleAccountStatus(em, "authorized", "已拉起授权 (Kiro Web)")

	go func() {
		time.Sleep(5 * time.Second)
		automation.SyncAccountLoginStatus(em)
	}()

	return map[string]interface{}{"success": true}
}

// MarkGoogleAuthorized 手动标记 Google 账号已完成 Kiro 授权
func (a *App) MarkGoogleAuthorized(em string) map[string]interface{} {
	if err := storage.UpdateGoogleAccountStatus(em, "authorized", "已标记为已授权"); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true}
}

// AutoLoginGoogleAccount 异步执行指定 Google 账号的全要素自动登录
func (a *App) AutoLoginGoogleAccount(em string, autoAuthorizeKiro bool) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	for i := range accounts {
		if accounts[i].Email == em {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return map[string]interface{}{"error": "未找到指定账号"}
	}

	unlock, err := a.lockGoogleAccount(em, "自动登录")
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	go func() {
		defer unlock()
		_ = storage.UpdateGoogleAccountStatus(em, "logging_in", "正在自动登录...")
		runtime.EventsEmit(a.ctx, "google_login_progress", map[string]interface{}{
			"email":   em,
			"message": "正在初始化独立 Chrome 环境...",
			"status":  "logging_in",
		})

		err := automation.AutoLoginGoogleAccount(*target, autoAuthorizeKiro, func(msg string) {
			runtime.EventsEmit(a.ctx, "google_login_progress", map[string]interface{}{
				"email":   em,
				"message": msg,
				"status":  "logging_in",
			})
		})

		if err != nil {
			_ = storage.UpdateGoogleAccountStatus(em, "error", fmt.Sprintf("登录失败: %v", err))
			runtime.EventsEmit(a.ctx, "google_login_progress", map[string]interface{}{
				"email":   em,
				"message": fmt.Sprintf("登录失败: %v", err),
				"status":  "error",
			})
		} else {
			finalStatus := "logged_in"
			finalMsg := "已就绪 (本地会话有效)"
			for _, updatedAcc := range storage.GetGoogleAccounts() {
				if strings.EqualFold(updatedAcc.Email, em) && (updatedAcc.Status == "authorized" || updatedAcc.Status == "logged_in") {
					finalStatus = updatedAcc.Status
					if updatedAcc.StatusMsg != "" {
						finalMsg = updatedAcc.StatusMsg
					}
					break
				}
			}
			_ = storage.UpdateGoogleAccountStatus(em, finalStatus, finalMsg)
			runtime.EventsEmit(a.ctx, "google_login_progress", map[string]interface{}{
				"email":   em,
				"message": finalMsg,
				"status":  finalStatus,
			})
		}
	}()

	return map[string]interface{}{"success": true}
}

// BatchAutoLoginGoogleAccounts 批量自动登录所有未就绪的 Google 账号
func (a *App) BatchAutoLoginGoogleAccounts(autoAuthorizeKiro bool) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var targets []storage.GoogleAccount
	for _, acc := range accounts {
		if acc.Status != "logged_in" && acc.Status != "authorized" {
			targets = append(targets, acc)
		}
	}

	if len(targets) == 0 {
		return map[string]interface{}{"error": "没有需要自动登录的未就绪账号"}
	}

	go func() {
		consecutiveRapidFailures := 0
		for idx, target := range targets {
			em := target.Email
			unlock, err := a.lockGoogleAccount(em, "批量自动登录")
			if err != nil {
				log.Printf("[BatchAutoLogin] 跳过忙碌账号 %s: %v", em, err)
				continue
			}

			_ = storage.UpdateGoogleAccountStatus(em, "logging_in", "正在自动登录...")
			runtime.EventsEmit(a.ctx, "google_login_progress", map[string]interface{}{
				"email":   em,
				"message": "正在拉起环境自动登录...",
				"status":  "logging_in",
			})

			startT := time.Now()
			loginErr := automation.AutoLoginGoogleAccountWithLayout(target, autoAuthorizeKiro, func(msg string) {
				runtime.EventsEmit(a.ctx, "google_login_progress", map[string]interface{}{
					"email":   em,
					"message": msg,
					"status":  "logging_in",
				})
			}, idx, 1)

			unlock()

			if loginErr != nil {
				if time.Since(startT) < 6*time.Second {
					consecutiveRapidFailures++
				} else {
					consecutiveRapidFailures = 1
				}
				if consecutiveRapidFailures >= 3 {
					msg := "⚠️ 检测到连续 3 个账号极速启动失败，疑似代理或网络故障，已自动熔断暂停后续批量登录！"
					log.Printf("[BatchAutoLogin] %s", msg)
					runtime.EventsEmit(a.ctx, "google_login_progress", map[string]interface{}{
						"email":   em,
						"message": msg,
						"status":  "error",
					})
					break
				}
			} else {
				consecutiveRapidFailures = 0
			}

			time.Sleep(2 * time.Second)
		}
	}()

	return map[string]interface{}{"success": true, "count": len(targets)}
}

// SyncGoogleAccountsStatus 批量同步本地 Google 账号环境状态
func (a *App) SyncGoogleAccountsStatus() map[string]interface{} {
	automation.SyncAllGoogleAccountsStatus()
	return map[string]interface{}{
		"success":  true,
		"accounts": storage.GetGoogleAccounts(),
	}
}

// GetGoogleTOTP 计算 2FA 动态验证码
func (a *App) GetGoogleTOTP(secret string) map[string]interface{} {
	code, err := storage.GenerateTOTPCode(secret)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "code": code}
}

// VerifyTOTPSecret 实时验证 Base32 密钥并返回当前 6 位动态码与倒计时剩余秒数
func (a *App) VerifyTOTPSecret(secret string) map[string]interface{} {
	code, err := storage.GenerateTOTPCode(secret)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	remainingSeconds := 30 - (time.Now().Unix() % 30)
	return map[string]interface{}{
		"success":   true,
		"valid":     true,
		"code":      code,
		"remaining": remainingSeconds,
	}
}

// OpenGoogleSecurityCenter 拉起指定 Google 账号独立 Profile 并直达两步验证/安全中心设置页
func (a *App) OpenGoogleSecurityCenter(em string) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	for i := range accounts {
		if accounts[i].Email == em {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return map[string]interface{}{"error": "未找到指定账号"}
	}

	if a.IsGoogleAccountBusy(em) {
		return map[string]interface{}{"error": fmt.Sprintf("账号 [%s] 正在执行后台自动化任务，请稍后再操作安全中心", em)}
	}

	securityURL := "https://myaccount.google.com/two-step-verification/authenticator"
	if err := automation.LaunchInteractiveProfile(*target, securityURL); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	return map[string]interface{}{"success": true}
}

// UpdateGoogleAccountManual 手动更新账号凭据 (密码、辅助邮箱、2FA密钥、代理)
func (a *App) UpdateGoogleAccountManual(email, password, recovery, totpSecret, proxy string) map[string]interface{} {
	if err := storage.UpdateGoogleAccountCredentials(email, password, recovery, totpSecret, proxy); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true}
}

// UpdateGoogleAccount2FAManual 手动更新账号的 2FA 密钥（自动备份原密钥，不影响密码/辅助邮箱/代理）
func (a *App) UpdateGoogleAccount2FAManual(email, totpSecret string) map[string]interface{} {
	if err := storage.UpdateGoogleAccount2FA(email, totpSecret); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true}
}

// ChangeGoogle2FAAuto 启动后台全自动修改指定账号的 Google 2FA 身份验证器密钥
func (a *App) ChangeGoogle2FAAuto(em string) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	for i := range accounts {
		if accounts[i].Email == em {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return map[string]interface{}{"error": "未找到指定账号"}
	}

	unlock, err := a.lockGoogleAccount(em, "修改2FA")
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	go func() {
		defer unlock()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Change2FA][PANIC RECOVERED] 账号 %s 发生未捕获异常: %v", em, r)
				runtime.EventsEmit(a.ctx, "google_2fa_progress", map[string]interface{}{
					"email":   em,
					"message": fmt.Sprintf("❌ 异常中断: %v", r),
					"status":  "error",
				})
			}
		}()

		runtime.EventsEmit(a.ctx, "google_2fa_progress", map[string]interface{}{
			"email":   em,
			"message": "正在启动独立 Chrome 环境并进入两步验证中心...",
			"status":  "running",
		})

		newSecret, err := automation.AutomateChangeGoogle2FA(*target, func(msg string) {
			runtime.EventsEmit(a.ctx, "google_2fa_progress", map[string]interface{}{
				"email":   em,
				"message": msg,
				"status":  "running",
			})
		})

		if err != nil {
			runtime.EventsEmit(a.ctx, "google_2fa_progress", map[string]interface{}{
				"email":   em,
				"message": fmt.Sprintf("修改 2FA 失败: %v", err),
				"status":  "error",
			})
		} else {
			runtime.EventsEmit(a.ctx, "google_2fa_progress", map[string]interface{}{
				"email":     em,
				"message":   "✅ 2FA 密钥已成功在谷歌安全中心重置并激活！",
				"status":    "success",
				"newSecret": newSecret,
			})
		}
	}()

	return map[string]interface{}{"success": true}
}

// BatchChangeGoogle2FAAuto 批量全自动修改多个指定 Google 账号的 2FA 密钥
func (a *App) BatchChangeGoogle2FAAuto(emails []string) map[string]interface{} {
	if len(emails) == 0 {
		return map[string]interface{}{"error": "请选择至少一个账号"}
	}

	accounts := storage.GetGoogleAccounts()
	accMap := make(map[string]storage.GoogleAccount)
	for _, acc := range accounts {
		accMap[acc.Email] = acc
	}

	var targets []storage.GoogleAccount
	for _, em := range emails {
		if acc, ok := accMap[em]; ok {
			targets = append(targets, acc)
		}
	}

	if len(targets) == 0 {
		return map[string]interface{}{"error": "未找到有效账号"}
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[BatchChange2FA][PANIC RECOVERED] 批量队列发生未捕获异常: %v", r)
				runtime.EventsEmit(a.ctx, "google_batch_2fa_progress", map[string]interface{}{
					"message": fmt.Sprintf("❌ 批量队列异常中断: %v", r),
					"status":  "error",
				})
			}
		}()

		consecutiveRapidFailures := 0
		for idx, target := range targets {
			em := target.Email
			unlock, err := a.lockGoogleAccount(em, "批量修改2FA")
			if err != nil {
				runtime.EventsEmit(a.ctx, "google_batch_2fa_progress", map[string]interface{}{
					"email":   em,
					"index":   idx + 1,
					"total":   len(targets),
					"message": fmt.Sprintf("⚠️ 跳过繁忙账号: %v", err),
					"status":  "running",
				})
				continue
			}

			runtime.EventsEmit(a.ctx, "google_batch_2fa_progress", map[string]interface{}{
				"email":   em,
				"index":   idx + 1,
				"total":   len(targets),
				"message": fmt.Sprintf("[%d/%d] 正在为 %s 启动环境修改 2FA...", idx+1, len(targets), em),
				"status":  "running",
			})

			startT := time.Now()
			newSecret, err := automation.AutomateChangeGoogle2FAWithLayout(target, func(msg string) {
				runtime.EventsEmit(a.ctx, "google_batch_2fa_progress", map[string]interface{}{
					"email":   em,
					"index":   idx + 1,
					"total":   len(targets),
					"message": msg,
					"status":  "running",
				})
			}, idx, 1)

			unlock()

			if err != nil {
				if time.Since(startT) < 6*time.Second {
					consecutiveRapidFailures++
				} else {
					consecutiveRapidFailures = 1
				}
				runtime.EventsEmit(a.ctx, "google_batch_2fa_progress", map[string]interface{}{
					"email":   em,
					"index":   idx + 1,
					"total":   len(targets),
					"message": fmt.Sprintf("❌ 修改失败: %v", err),
					"status":  "error",
				})
				if consecutiveRapidFailures >= 3 {
					msg := "⚠️ 检测到连续 3 个账号极速启动失败，疑似代理或网络异常，已自动熔断暂停后续批量修改任务！"
					runtime.EventsEmit(a.ctx, "google_batch_2fa_progress", map[string]interface{}{
						"message": msg,
						"status":  "error",
					})
					break
				}
			} else {
				consecutiveRapidFailures = 0
				runtime.EventsEmit(a.ctx, "google_batch_2fa_progress", map[string]interface{}{
					"email":     em,
					"index":     idx + 1,
					"total":     len(targets),
					"message":   "✅ 2FA 修改成功并已持久化",
					"status":    "item_success",
					"newSecret": newSecret,
				})
			}
			time.Sleep(2 * time.Second)
		}

		runtime.EventsEmit(a.ctx, "google_batch_2fa_progress", map[string]interface{}{
			"message": "🎉 批量 2FA 修改任务执行完毕！",
			"status":  "batch_done",
		})
	}()

	return map[string]interface{}{"success": true, "count": len(targets)}
}

// GetGoogleKiroCredentials 获取指定账号格式化后的 Kiro 凭据数据与 JSON (format: "kam" 或 "ide")
func (a *App) GetGoogleKiroCredentials(email, format string) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	for i := range accounts {
		if strings.EqualFold(accounts[i].Email, strings.TrimSpace(email)) {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return map[string]interface{}{"error": "未找到指定账号"}
	}

	hasCreds := strings.TrimSpace(target.KiroRefreshToken) != ""
	var jsonStr string
	if hasCreds {
		s, err := storage.BuildKiroCredentialJSON(*target, format)
		if err == nil {
			jsonStr = s
		}
	}

	return map[string]interface{}{
		"success":        true,
		"email":          target.Email,
		"hasCredentials": hasCreds,
		"hasCreds":       hasCreds,
		"refreshToken":   target.KiroRefreshToken,
		"accessToken":    target.KiroAccessToken,
		"profileArn":     target.KiroProfileArn,
		"userId":         target.KiroUserId,
		"expiresAt":      target.KiroExpiresAt,
		"updatedAt":      target.KiroUpdatedAt,
		"json":           jsonStr,
	}
}

// ExtractGoogleKiroCredentials 主动拉起独立 Chrome Profile 从 app.kiro.dev 读取 Session Cookies 并刷新补全 ProfileArn
func (a *App) ExtractGoogleKiroCredentials(email string) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	for i := range accounts {
		if strings.EqualFold(accounts[i].Email, strings.TrimSpace(email)) {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return map[string]interface{}{"error": "未找到指定账号"}
	}

	unlock, err := a.lockGoogleAccount(target.Email, "提取Kiro凭据")
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	defer unlock()

	updated, err := automation.ExtractKiroSessionFromProfile(*target, func(msg string) {
		runtime.EventsEmit(a.ctx, "google_kiro_creds_progress", map[string]interface{}{
			"email":   target.Email,
			"message": msg,
		})
	})
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	kamJSON, _ := storage.BuildKiroCredentialJSON(*updated, "kam")
	ideJSON, _ := storage.BuildKiroCredentialJSON(*updated, "ide")

	return map[string]interface{}{
		"success":        true,
		"email":          updated.Email,
		"hasCredentials": true,
		"hasCreds":       true,
		"refreshToken":   updated.KiroRefreshToken,
		"accessToken":    updated.KiroAccessToken,
		"profileArn":     updated.KiroProfileArn,
		"userId":         updated.KiroUserId,
		"expiresAt":      updated.KiroExpiresAt,
		"updatedAt":      updated.KiroUpdatedAt,
		"kamJson":        kamJSON,
		"ideJson":        ideJSON,
	}
}

// BatchExportGoogleKiroCredentials 批量导出指定或所有具备凭据的 Google 账号为 JSON 文件 (format: "kam" 或 "ide")
func (a *App) BatchExportGoogleKiroCredentials(emails []string, format string) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	emailSet := make(map[string]bool)
	for _, e := range emails {
		emailSet[strings.ToLower(strings.TrimSpace(e))] = true
	}

	var targets []storage.GoogleAccount
	for _, acc := range accounts {
		if len(emailSet) > 0 && !emailSet[strings.ToLower(strings.TrimSpace(acc.Email))] {
			continue
		}
		if strings.TrimSpace(acc.KiroRefreshToken) != "" {
			targets = append(targets, acc)
		}
	}

	if len(targets) == 0 {
		return map[string]interface{}{"error": "未找到包含有效 Kiro 凭据的账号，请先执行登录或凭据提取"}
	}

	jsonStr, err := storage.BuildBatchKiroCredentialsJSON(targets, format)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	defaultName := fmt.Sprintf("kiro-accounts-%s.json", time.Now().Format("20060102-150405"))
	savePath, _ := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出 Kiro 凭据 JSON 文件",
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON 文件 (*.json)", Pattern: "*.json"},
		},
	})

	if savePath != "" {
		if err := os.WriteFile(savePath, []byte(jsonStr), 0600); err != nil {
			return map[string]interface{}{"error": fmt.Sprintf("写入文件失败: %v", err)}
		}
		return map[string]interface{}{
			"success": true,
			"count":   len(targets),
			"path":    savePath,
			"json":    jsonStr,
			"content": jsonStr,
		}
	}

	return map[string]interface{}{
		"success":   true,
		"cancelled": true,
		"count":     len(targets),
		"json":      jsonStr,
		"content":   jsonStr,
	}
}

// SelectGoogleFile 选择 Google 账号导入文件
func (a *App) SelectGoogleFile() string {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Google 账号文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "文本文件 (*.txt)", Pattern: "*.txt"},
			{DisplayName: "CSV 文件 (*.csv)", Pattern: "*.csv"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		log.Printf("选择文件失败: %v", err)
		return ""
	}
	return path
}

// GetSupportedSubscriptionPlans 获取支持的订阅套餐列表
func (a *App) GetSupportedSubscriptionPlans() []automation.PlanInfo {
	return automation.GetSupportedPlans()
}

// GenerateGoogleSubscriptionLink 为指定 Google 账号生成长时效 Stripe 订阅付款链接
func (a *App) GenerateGoogleSubscriptionLink(em string, planType string) map[string]interface{} {
	accounts := storage.GetGoogleAccounts()
	var target *storage.GoogleAccount
	for i := range accounts {
		if accounts[i].Email == em {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return map[string]interface{}{"error": "未找到指定账号"}
	}

	unlock, err := a.lockGoogleAccount(em, "生成付款链接")
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	go func() {
		defer unlock()
		runtime.EventsEmit(a.ctx, "google_payment_progress", map[string]interface{}{
			"email":   em,
			"message": fmt.Sprintf("正在拉起环境生成 [%s] 付款链接...", planType),
			"status":  "generating",
		})

		stripeURL, err := automation.GenerateSubscriptionPaymentURL(*target, planType, func(msg string) {
			runtime.EventsEmit(a.ctx, "google_payment_progress", map[string]interface{}{
				"email":   em,
				"message": msg,
				"status":  "generating",
			})
		})

		if err != nil {
			runtime.EventsEmit(a.ctx, "google_payment_progress", map[string]interface{}{
				"email":   em,
				"message": fmt.Sprintf("生成失败: %v", err),
				"status":  "error",
			})
		} else {
			runtime.EventsEmit(a.ctx, "google_payment_progress", map[string]interface{}{
				"email":      em,
				"message":    "🎉 成功生成长时效 Stripe 结账链接！",
				"status":     "generated",
				"paymentUrl": stripeURL,
			})
		}
	}()

	return map[string]interface{}{"success": true}
}

// BatchGenerateGoogleSubscriptionLinks 批量为 Google 账号生成订阅付款链接
func (a *App) BatchGenerateGoogleSubscriptionLinks(emails []string, planType string, concurrency int) map[string]interface{} {
	allAccounts := storage.GetGoogleAccounts()
	var targets []storage.GoogleAccount
	emailMap := make(map[string]bool)
	for _, em := range emails {
		emailMap[strings.ToLower(strings.TrimSpace(em))] = true
	}

	for _, acc := range allAccounts {
		if len(emails) == 0 || emailMap[strings.ToLower(acc.Email)] {
			targets = append(targets, acc)
		}
	}

	if len(targets) == 0 {
		return map[string]interface{}{"error": "没有可供生成的账号"}
	}

	go func() {
		var availableTargets []storage.GoogleAccount
		var unlocks []func()

		for _, acc := range targets {
			u, err := a.lockGoogleAccount(acc.Email, "批量生成付款链接")
			if err != nil {
				log.Printf("[BatchSubLinks] 跳过繁忙账号 %s: %v", acc.Email, err)
				continue
			}
			availableTargets = append(availableTargets, acc)
			unlocks = append(unlocks, u)
		}

		if len(availableTargets) == 0 {
			runtime.EventsEmit(a.ctx, "google_payment_progress", map[string]interface{}{
				"message": "所选账号当前正在执行其他任务，批量任务已跳过",
				"status":  "batch_done",
			})
			return
		}

		defer func() {
			for _, u := range unlocks {
				u()
			}
		}()

		_, _ = automation.BatchGenerateSubscriptionLinks(availableTargets, planType, concurrency, func(msg string) {
			runtime.EventsEmit(a.ctx, "google_payment_progress", map[string]interface{}{
				"message": msg,
				"status":  "batch_generating",
			})
		})
		runtime.EventsEmit(a.ctx, "google_payment_progress", map[string]interface{}{
			"message": "批量付款链接生成任务已全部结束！",
			"status":  "batch_done",
		})
	}()

	return map[string]interface{}{"success": true, "count": len(targets)}
}

// AutofillStripePayment 打开 Stripe 链接并仿真自动填写卡片与账单表单
func (a *App) AutofillStripePayment(paymentURL string, card automation.CreditCardInfo, autoSubmit bool) map[string]interface{} {
	go func() {
		runtime.EventsEmit(a.ctx, "stripe_autofill_progress", map[string]interface{}{
			"message": "正在拉起浏览器并导航至 Stripe 付款页面...",
			"status":  "starting",
		})

		err := automation.AutoFillStripeCheckout(paymentURL, card, autoSubmit, func(msg string) {
			runtime.EventsEmit(a.ctx, "stripe_autofill_progress", map[string]interface{}{
				"message": msg,
				"status":  "filling",
			})
		})

		if err != nil {
			runtime.EventsEmit(a.ctx, "stripe_autofill_progress", map[string]interface{}{
				"message": fmt.Sprintf("填写失败: %v", err),
				"status":  "error",
			})
		} else {
			runtime.EventsEmit(a.ctx, "stripe_autofill_progress", map[string]interface{}{
				"message": "🎉 表单自动填写完毕！",
				"status":  "done",
			})
		}
	}()

	return map[string]interface{}{"success": true}
}

// ImportGoogleFile 从文件导入 Google 账号
func (a *App) ImportGoogleFile(filePath string) map[string]interface{} {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("读取文件失败: %v", err)}
	}
	return storage.AddGoogleAccounts(string(content))
}


// SelectDirectory 选择目录 (Wails Dialog)
func (a *App) SelectDirectory() string {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择目录",
	})
	if err != nil {
		log.Printf("选择目录失败: %v", err)
		return ""
	}
	return path
}

// SelectOutlookFile 选择 Outlook 账号文件 (Wails Dialog)
func (a *App) SelectOutlookFile() string {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Outlook 账号文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "文本文件 (*.txt)",
				Pattern:     "*.txt",
			},
			{
				DisplayName: "CSV 文件 (*.csv)",
				Pattern:     "*.csv",
			},
			{
				DisplayName: "所有文件 (*.*)",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		log.Printf("选择文件失败: %v", err)
		return ""
	}
	return path
}

// SelectICloudFile 选择 iCloud 账号文件 (Wails Dialog)
func (a *App) SelectICloudFile() string {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 iCloud 账号文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "CSV 文件 (*.csv)",
				Pattern:     "*.csv",
			},
			{
				DisplayName: "文本文件 (*.txt)",
				Pattern:     "*.txt",
			},
			{
				DisplayName: "所有文件 (*.*)",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		log.Printf("选择文件失败: %v", err)
		return ""
	}
	return path
}

// GetDataDir 前端获取当前存储目录
func (a *App) GetDataDir() string {
	return storage.GetDataDir()
}

// SetDataDir 设置自定义存储目录（自动迁移旧数据）
func (a *App) SetDataDir(dir string) map[string]interface{} {
	path, err := storage.SetDataDirPath(dir)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	proxy.InitPool(path)
	return map[string]interface{}{"success": true, "path": path}
}

// ResetDataDir 重置为默认存储目录
func (a *App) ResetDataDir() map[string]interface{} {
	path, err := storage.ResetDataDirPath()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	proxy.InitPool(path)
	return map[string]interface{}{"success": true, "path": path}
}

// GetResultOutputDir 获取注册结果输出目录（明文 accounts.json 的写入位置）
func (a *App) GetResultOutputDir() string {
	return storage.GetResultOutputDir()
}

// SetResultOutputDir 设置注册结果输出目录
func (a *App) SetResultOutputDir(dir string) map[string]interface{} {
	path, err := storage.SetResultOutputDir(dir)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "path": path}
}

// ResetResultOutputDir 重置为默认输出目录
func (a *App) ResetResultOutputDir() map[string]interface{} {
	path, err := storage.ResetResultOutputDir()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "path": path}
}

// GetProxy 返回当前全局代理（空字符串=直连）
func (a *App) GetProxy() string {
	return storage.GetProxy()
}

// SetProxy 保存全局代理；输入的简写（host:port:user:pass 等）会被自动归一化；
// 保存后会探测代理出口 IP 与归属信息并一并返回。
func (a *App) SetProxy(raw string) map[string]interface{} {
	normalized, err := storage.SetProxy(raw)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	resp := map[string]interface{}{"success": true, "proxy": normalized}
	if normalized != "" {
		resp["detect"] = proxy.Detect(normalized)
	}
	return resp
}

// DetectProxy 单独探测一个代理（不保存），用于"测试连接"
func (a *App) DetectProxy(raw string) proxy.Info {
	normalized := storage.NormalizeProxyAddress(raw)
	return proxy.Detect(normalized)
}

// ResetProxy 清空代理，恢复直连
func (a *App) ResetProxy() map[string]interface{} {
	storage.ResetProxy()
	return map[string]interface{}{"success": true}
}

// GetLanguage 获取当前界面语言代码，空字符串表示未设置（前端应回落到 OS 语言）
func (a *App) GetLanguage() string {
	return storage.GetLanguage()
}

// GetAppSettings returns the unified task, interface, notification and advanced settings.
func (a *App) GetAppSettings() storage.AppSettings {
	return storage.GetAppSettings()
}

// SaveAppSettings validates and persists all runtime settings atomically.
func (a *App) SaveAppSettings(settings storage.AppSettings) map[string]interface{} {
	saved, err := storage.SaveAppSettings(settings)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	storage.ConfigurePersistentLogs(saved.PersistentLogs, saved.LogRetentionDays)
	return map[string]interface{}{"success": true, "settings": saved}
}

func (a *App) GetLogsDir() string {
	return storage.GetLogsDir()
}

func (a *App) OpenLogsDir() {
	path := storage.GetLogsDir()
	runtime.BrowserOpenURL(a.ctx, (&url.URL{Scheme: "file", Path: path}).String())
}

func (a *App) ClearLogs() map[string]interface{} {
	if err := storage.ClearLogs(); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true}
}

// SetLanguage 保存界面语言；仅接受 "zh"/"en"/"ja"
func (a *App) SetLanguage(lang string) map[string]interface{} {
	if err := storage.SetLanguage(lang); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "language": lang}
}

// GetOSLanguage 返回操作系统语言代码 "zh"/"en"/"ja"，用于首次启动自动选语言
func (a *App) GetOSLanguage() string {
	return detectOSLanguage()
}

// StartTask 启动注册任务
func (a *App) StartTask(req task.StartTaskRequest) map[string]interface{} {
	return task.StartTask(req)
}

// StopTask 停止注册任务
func (a *App) StopTask() map[string]interface{} {
	return task.StopTask(true)
}

// CheckUpdate 手动检查更新
func (a *App) CheckUpdate() map[string]interface{} {
	return updater.CheckUpdate()
}

// ResetFingerprintCache 清空所有按代理缓存的浏览器指纹，下一次注册重新生成
func (a *App) ResetFingerprintCache() map[string]interface{} {
	browser.ResetIdentityCache()
	return map[string]interface{}{"success": true}
}

// ---- 多代理池 ----

// ListProxyPool 返回当前代理池
func (a *App) ListProxyPool() []proxy.PoolEntry {
	return proxy.List()
}

// AddProxyEntry 新增一条代理（url 会被归一化），weight 1-100
func (a *App) AddProxyEntry(name, rawURL string, weight int) map[string]interface{} {
	normalized := storage.NormalizeProxyAddress(rawURL)
	entry, err := proxy.Add(proxy.PoolEntry{Name: name, URL: normalized, Weight: weight})
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "entry": entry}
}

// UpdateProxyEntry 更新代理（按 id）
func (a *App) UpdateProxyEntry(id, name, rawURL string, weight int, enabled bool) map[string]interface{} {
	var u string
	if rawURL != "" {
		u = storage.NormalizeProxyAddress(rawURL)
	}
	entry, err := proxy.Update(id, proxy.PoolEntry{Name: name, URL: u, Weight: weight, Enabled: enabled})
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "entry": entry}
}

// DeleteProxyEntry 删除（按 id）
func (a *App) DeleteProxyEntry(id string) map[string]interface{} {
	if err := proxy.Delete(id); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true}
}

// TestProxyByID 测试某条代理（按 id），并把探测结果持久化到代理池
func (a *App) TestProxyByID(id string) map[string]interface{} {
	entry, err := proxy.Get(id)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	start := time.Now()
	info := proxy.Detect(entry.URL)
	ms := int(time.Since(start) / time.Millisecond)
	_ = proxy.SetProbe(id, info, ms)
	return map[string]interface{}{
		"ok": info.OK, "scheme": info.Scheme, "ip": info.IP,
		"country": info.Country, "countryCode": info.CountryCode,
		"region": info.Region, "city": info.City,
		"isp": info.ISP, "proxyType": info.ProxyType,
		"error": info.Error, "ms": ms,
	}
}
