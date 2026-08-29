package wsclient

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestTunnelCommandIsIsolatedFromTerminalSignals(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	isolateTunnelCommandFromTerminalSignals(cmd)
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("SysProcAttr = %#v, want a separate process group", cmd.SysProcAttr)
	}
}
