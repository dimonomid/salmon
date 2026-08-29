//go:build unix

package wsclient

import (
	"os/exec"
	"testing"
)

func TestTunnelCommandIsIsolatedFromTerminalSignals(t *testing.T) {
	cmd := exec.Command("true")
	isolateTunnelCommandFromTerminalSignals(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("SysProcAttr = %#v, want a separate process group", cmd.SysProcAttr)
	}
}
