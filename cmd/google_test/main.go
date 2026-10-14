package main

import (
	"flag"
	"log"
	"os"
	"strings"
	"sync"

	"reg_go/internal/browser/automation"
	"reg_go/internal/storage"
)

func main() {
	filePath := flag.String("file", `D:\git\KiroX\测试账号.txt`, "Path to accounts file")
	concurrency := flag.Int("concurrency", 1, "Concurrent worker count")
	autoAuthorizeKiro := flag.Bool("kiro", true, "Auto authorize Kiro after login")
	change2FA := flag.Bool("change2fa", false, "Test 2FA change instead of login")
	accountIndex := flag.Int("index", -1, "Run specific account index (0-based, -1 for all)")
	proxyFlag := flag.String("proxy", "", "Proxy URL (optional, e.g. http://127.0.0.1:7890)")
	flag.Parse()

	log.Printf("==== Google 账号全流程集成测试 ====")
	log.Printf("读取账号文件: %s", *filePath)

	// 1. 设置全局代理 (若传入了-proxy则设置；若原配置中是失效的17840则清理)
	if *proxyFlag != "" {
		_, _ = storage.SetProxy(*proxyFlag)
	} else if storage.GetProxy() == "http://127.0.0.1:17840" {
		_, _ = storage.SetProxy("")
	}
	log.Printf("全局代理配置为: %q (空表示直连/系统代理/WARP)", storage.GetProxy())

	// 2. 读取并导入账号
	data, err := os.ReadFile(*filePath)
	if err != nil {
		log.Fatalf("读取测试账号文件失败: %v", err)
	}

	res := storage.AddGoogleAccounts(string(data))
	log.Printf("导入结果: %+v", res)

	accounts := storage.GetGoogleAccounts()
	log.Printf("存储库中现有 %d 个 Google 账号:", len(accounts))
	for i, acc := range accounts {
		log.Printf("  [%d] Email: %s, Pass: %s, Recovery: %s, TOTP: %s, Proxy: %s, Status: %s",
			i, acc.Email, maskStr(acc.Password), acc.Recovery, acc.TOTPSecret, acc.Proxy, acc.Status)
	}

	if len(accounts) == 0 {
		log.Fatalf("没有可用账号进行测试")
	}

	var targets []storage.GoogleAccount
	if *accountIndex >= 0 && *accountIndex < len(accounts) {
		targets = append(targets, accounts[*accountIndex])
	} else {
		targets = accounts
	}

	log.Printf("\n==== 开始启动自动化登录流程 (目标数: %d, 并发数: %d) ====", len(targets), *concurrency)

	var wg sync.WaitGroup
	sem := make(chan struct{}, *concurrency)

	for i, acc := range targets {
		wg.Add(1)
		sem <- struct{}{}

		workerIdx := i % (*concurrency)
		go func(idx int, target storage.GoogleAccount, wIdx int) {
			defer wg.Done()
			defer func() { <-sem }()

			log.Printf("🚀 [Worker %d] 开始执行账号: %s", wIdx, target.Email)
			var err error
			if *change2FA {
				log.Printf("🔑 [Worker %d] 正在启动 2FA 修改测试...", wIdx)
				newSecret, e := automation.AutomateChangeGoogle2FAWithLayout(target, func(msg string) {
					log.Printf("📢 [%s] %s", target.Email, msg)
				}, wIdx, *concurrency)
				err = e
				if e == nil {
					log.Printf("🎉 [%s] 成功获取并激活新 2FA 密钥: %s", target.Email, newSecret)
				}
			} else {
				err = automation.AutoLoginGoogleAccountWithLayout(target, *autoAuthorizeKiro, func(msg string) {
					log.Printf("📢 [%s] %s", target.Email, msg)
				}, wIdx, *concurrency)
			}

			if err != nil {
				log.Printf("❌ [%s] 流程执行失败: %v", target.Email, err)
			} else {
				log.Printf("✅ [%s] 流程顺利完成！", target.Email)
			}
		}(i, acc, workerIdx)
	}

	wg.Wait()
	log.Printf("\n==== 全部流程测试完毕 ====")
}

func maskStr(s string) string {
	if len(s) <= 3 {
		return "***"
	}
	return s[:2] + strings.Repeat("*", len(s)-2)
}
