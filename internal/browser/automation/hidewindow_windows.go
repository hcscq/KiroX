//go:build windows

package automation

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow 避免 GUI 模式下拉起 PowerShell 时闪现黑框
func hideConsoleWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
