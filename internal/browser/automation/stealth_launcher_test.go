package automation

import (
	"strings"
	"testing"
)

func TestBuildLaunchArgs(t *testing.T) {
	profileDir := `C:\temp\kirox\profile_test`
	proxy := "http://127.0.0.1:7890"
	targetURL := "https://accounts.google.com"

	args := BuildLaunchArgs(profileDir, proxy, targetURL)

	hasProfile := false
	hasProxy := false
	hasAntiDetect := false
	hasURL := false

	for _, a := range args {
		if strings.Contains(a, "--user-data-dir="+profileDir) {
			hasProfile = true
		}
		if strings.Contains(a, "--proxy-server=http://127.0.0.1:7890") {
			hasProxy = true
		}
		if a == "--disable-blink-features=AutomationControlled" {
			hasAntiDetect = true
		}
		if a == targetURL {
			hasURL = true
		}
	}

	if !hasProfile {
		t.Errorf("缺少 --user-data-dir 参数: %v", args)
	}
	if !hasProxy {
		t.Errorf("缺少 --proxy-server 参数: %v", args)
	}
	if !hasAntiDetect {
		t.Errorf("缺少反检测参数: %v", args)
	}
	if !hasURL {
		t.Errorf("缺少目标 URL: %v", args)
	}
}

func TestFindBrowserPath(t *testing.T) {
	path, err := FindBrowserPath()
	if err != nil {
		t.Logf("本机可能未安装默认路径 Chrome/Edge: %v", err)
		return
	}
	if path == "" {
		t.Errorf("未找到有效浏览器路径")
	}
	t.Logf("找到浏览器路径: %s", path)
}

func TestIsLoginSuccessURL(t *testing.T) {
	testCases := []struct {
		url      string
		expected bool
	}{
		{"https://myaccount.google.com/?nlr=1", true},
		{"https://mail.google.com/mail/u/0/#inbox", true},
		{"https://accounts.google.com/b/0/AddSession", true},
		{"https://app.kiro.dev/signin/oauth", true},
		{"https://accounts.google.com/signin/v2/identifier", false},
		{"https://accounts.google.com/signin/v2/challenge/pwd", false},
	}

	for _, tc := range testCases {
		res := isLoginSuccessURL(tc.url)
		if res != tc.expected {
			t.Errorf("url: %s, expected: %v, got: %v", tc.url, tc.expected, res)
		}
	}
}

func TestCalculateWindowBounds(t *testing.T) {
	// 验证单窗口
	_, _, w, h := CalculateWindowBounds(0, 1)
	if w <= 0 || h <= 0 {
		t.Errorf("无效的单窗口尺寸: %dx%d", w, h)
	}

	// 验证双窗口不同位置
	x0, _, w0, _ := CalculateWindowBounds(0, 2)
	x1, _, w1, _ := CalculateWindowBounds(1, 2)
	if x0 == x1 {
		t.Errorf("双窗口应该错位: x0=%d, x1=%d", x0, x1)
	}
	if w0 != w1 {
		t.Errorf("双窗口宽度应该对称: w0=%d, w1=%d", w0, w1)
	}

	// 验证四宫格
	xTopLeft, yTopLeft, _, _ := CalculateWindowBounds(0, 4)
	xBottomRight, yBottomRight, _, _ := CalculateWindowBounds(3, 4)
	if xTopLeft >= xBottomRight || yTopLeft >= yBottomRight {
		t.Errorf("四宫格排版坐标异常: (0,4)=[%d,%d], (3,4)=[%d,%d]", xTopLeft, yTopLeft, xBottomRight, yBottomRight)
	}
}


