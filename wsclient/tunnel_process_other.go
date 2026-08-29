//go:build !unix && !windows

package wsclient

import "os/exec"

// isolateTunnelCommandFromTerminalSignals is a no-op on platforms without a
// supported process-group mechanism.
func isolateTunnelCommandFromTerminalSignals(_ *exec.Cmd) {}
