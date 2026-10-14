//go:build !windows

package automation

import "os/exec"

func hideConsoleWindow(cmd *exec.Cmd) {}
