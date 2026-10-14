package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoogleAccountsStorage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kirox_google_test_*")
	if err != nil {
		t.Fatalf("创建临时测试目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_dataDir = tempDir

	// 1. 测试批量导入
	raw := `
# 注释行
user1@gmail.com----pass123----rec1@gmail.com----JBSWY3DPEHPK3PXP----http://proxy1:8080
user2@gmail.com----pass456
user1@gmail.com----pass123-updated----rec1@gmail.com----JBSWY3DPEHPK3PXP
user3@gmail.com | pass789 | rec3@gmail.com
`
	res := AddGoogleAccounts(raw)
	if res["error"] != nil {
		t.Fatalf("AddGoogleAccounts 失败: %v", res["error"])
	}

	accounts := GetGoogleAccounts()
	if len(accounts) != 3 {
		t.Fatalf("期望 3 个账号，实际得到 %d 个", len(accounts))
	}

	// 验证更新
	if accounts[0].Email != "user1@gmail.com" || accounts[0].Password != "pass123-updated" {
		t.Errorf("user1 数据未正确更新: %+v", accounts[0])
	}
	if accounts[0].ProfileDir == "" || !filepath.IsAbs(accounts[0].ProfileDir) {
		t.Errorf("user1 ProfileDir 路径异常: %s", accounts[0].ProfileDir)
	}

	// 2. 测试状态更新
	err = UpdateGoogleAccountStatus("user1@gmail.com", "logged_in", "登录成功")
	if err != nil {
		t.Fatalf("UpdateGoogleAccountStatus 失败: %v", err)
	}
	accounts = GetGoogleAccounts()
	if accounts[0].Status != "logged_in" || accounts[0].StatusMsg != "登录成功" {
		t.Errorf("状态更新未生效: %+v", accounts[0])
	}

	// 3. 测试删除单个账号
	delRes := DeleteGoogleAccount("user2@gmail.com")
	if delRes["error"] != nil {
		t.Fatalf("DeleteGoogleAccount 失败: %v", delRes["error"])
	}
	accounts = GetGoogleAccounts()
	if len(accounts) != 2 {
		t.Fatalf("期望剩余 2 个账号，实际得到 %d 个", len(accounts))
	}
	// 验证 user3 解析
	if accounts[1].Email != "user3@gmail.com" || accounts[1].Password != "pass789" || accounts[1].Recovery != "rec3@gmail.com" {
		t.Errorf("user3 数据解析不符合预期: %+v", accounts[1])
	}

	// 4. 测试 2FA 独立修改与备份
	err = UpdateGoogleAccount2FA("user1@gmail.com", "MFRGGZDFMY========")
	if err != nil {
		t.Fatalf("UpdateGoogleAccount2FA 失败: %v", err)
	}
	accounts = GetGoogleAccounts()
	if accounts[0].TOTPSecret != "MFRGGZDFMY========" {
		t.Errorf("TOTPSecret 未正确更新: %s", accounts[0].TOTPSecret)
	}
	if accounts[0].BackupTOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Errorf("BackupTOTPSecret 未备份旧密钥: %s", accounts[0].BackupTOTPSecret)
	}
	if accounts[0].TOTPUpdatedAt == "" {
		t.Errorf("TOTPUpdatedAt 时间戳未生成")
	}

	// 5. 测试手动修改账号全部凭据
	err = UpdateGoogleAccountCredentials("user1@gmail.com", "newpass999", "newrec@gmail.com", "GEZDGNBVGY======", "http://127.0.0.1:1080")
	if err != nil {
		t.Fatalf("UpdateGoogleAccountCredentials 失败: %v", err)
	}
	accounts = GetGoogleAccounts()
	if accounts[0].Password != "newpass999" || accounts[0].Recovery != "newrec@gmail.com" || accounts[0].TOTPSecret != "GEZDGNBVGY======" || accounts[0].Proxy != "http://127.0.0.1:1080" {
		t.Errorf("凭据更新不符合预期: %+v", accounts[0])
	}
	if accounts[0].BackupTOTPSecret != "MFRGGZDFMY========" {
		t.Errorf("旧 2FA 密钥未被保存进备份: %s", accounts[0].BackupTOTPSecret)
	}

	// 6. 测试清空
	clearRes := ClearGoogleAccounts()
	if clearRes["error"] != nil {
		t.Fatalf("ClearGoogleAccounts 失败: %v", clearRes["error"])
	}
	accounts = GetGoogleAccounts()
	if len(accounts) != 0 {
		t.Fatalf("期望 0 个账号，实际得到 %d 个", len(accounts))
	}
}

func TestTOTPGeneration(t *testing.T) {
	// 验证 Base32 密钥生成 6 位纯数字验证码
	secret := "JBSWY3DPEHPK3PXP" // 标准示例密钥
	code, err := GenerateTOTPCode(secret)
	if err != nil {
		t.Fatalf("GenerateTOTPCode 失败: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("期望 6 位动态验证码，实际得到: %s", code)
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Fatalf("验证码包含非数字字符: %s", code)
		}
	}
}
