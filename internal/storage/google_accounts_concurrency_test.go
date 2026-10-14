package storage

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

// TestGoogleAccountsConcurrentUpdates 验证并发更新不同字段时不会发生丢更新 (Lost Update)
func TestGoogleAccountsConcurrentUpdates(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kirox_google_conc_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	_dataDir = tempDir
	resetGoogleAccountsCache()
	defer resetGoogleAccountsCache()

	const n = 20
	raw := ""
	for i := 0; i < n; i++ {
		raw += fmt.Sprintf("u%d@gmail.com----p%d\n", i, i)
	}
	if res := AddGoogleAccounts(raw); res["error"] != nil {
		t.Fatal(res["error"])
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		em := fmt.Sprintf("u%d@gmail.com", i)
		wg.Add(4)
		go func() { defer wg.Done(); _ = UpdateGoogleAccountStatus(em, "logged_in", "ok") }()
		go func() {
			defer wg.Done()
			_ = UpdateGoogleAccountPayment(em, "KIRO_PRO", "https://pay/"+em, "", "generated")
		}()
		go func() { defer wg.Done(); _ = UpdateGoogleAccountKiroTokens(em, "rt-"+em, "at", "arn", "uid", "") }()
		go func() { defer wg.Done(); _ = UpdateGoogleAccount2FA(em, "JBSWY3DPEHPK3PXP") }()
	}
	wg.Wait()

	// 清缓存后从磁盘重读，确认落盘结果完整
	resetGoogleAccountsCache()
	accs := GetGoogleAccounts()
	if len(accs) != n {
		t.Fatalf("账号数 %d != %d", len(accs), n)
	}
	for _, a := range accs {
		if a.Status != "logged_in" || a.PaymentURL != "https://pay/"+a.Email || a.KiroRefreshToken != "rt-"+a.Email || a.TOTPSecret != "JBSWY3DPEHPK3PXP" {
			t.Errorf("账号 %s 出现丢更新: %+v", a.Email, a)
		}
	}
}
