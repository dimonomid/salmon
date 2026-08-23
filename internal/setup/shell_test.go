package setup

import "testing"

func TestShellArgument(t *testing.T) {
	for _, test := range []struct {
		argument string
		want     string
	}{
		{"salmon-watch", "salmon-watch"},
		{"/tmp/my config.yml", "'/tmp/my config.yml'"},
		{"$HOME/salmon.yml", "'$HOME/salmon.yml'"},
		{"it's.yml", "'it'\"'\"'s.yml'"},
		{"", "''"},
	} {
		if got := ShellArgument(test.argument); got != test.want {
			t.Errorf("ShellArgument(%q) = %q, want %q", test.argument, got, test.want)
		}
	}
}
