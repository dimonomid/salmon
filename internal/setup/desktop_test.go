package setup

import "testing"

func TestDesktopExecArgument(t *testing.T) {
	argument := "a b%\"\\$`"
	if got, want := desktopExecArgument(argument), "\"a b%%\\\"\\\\\\\\\\\\$\\`\""; got != want {
		t.Fatalf("desktopExecArgument() = %q, want %q", got, want)
	}
}
