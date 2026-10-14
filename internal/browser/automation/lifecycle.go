package automation

import (
	"context"
	"sync"
)

// 浏览器自动化的父 Context：所有 chromedp 分配器都从这里派生，
// 以便 StopTask / 应用退出时能统一取消并回收后台 Chrome 实例。
var (
	baseMu     sync.Mutex
	baseParent = context.Background()
	baseCtx    context.Context
	baseCancel context.CancelFunc
)

func init() {
	baseCtx, baseCancel = context.WithCancel(baseParent)
}

// SetBaseContext 设置应用级父 Context (在 Wails startup 中调用)
func SetBaseContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	baseMu.Lock()
	defer baseMu.Unlock()
	oldCancel := baseCancel
	baseParent = ctx
	baseCtx, baseCancel = context.WithCancel(baseParent)
	if oldCancel != nil {
		oldCancel()
	}
}

// automationContext 返回当前自动化父 Context
func automationContext() context.Context {
	baseMu.Lock()
	defer baseMu.Unlock()
	return baseCtx
}

// CancelAllAutomation 取消所有正在运行的浏览器自动化，并为后续任务准备新的父 Context
func CancelAllAutomation() {
	baseMu.Lock()
	defer baseMu.Unlock()
	if baseCancel != nil {
		baseCancel()
	}
	baseCtx, baseCancel = context.WithCancel(baseParent)
}
