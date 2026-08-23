package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dimonomid/salmon/internal/setup"
)

func TestWatchConfigInitCreatesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "salmon-watch.yml")
	command := newWatchRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"config", "init", "--config", path})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), string(mustEmbeddedAsset("assets/setup/salmon-watch.yml")); got != want {
		t.Fatalf("config contents = %q, want %q", got, want)
	}
	if _, err := loadConfig(path); err != nil {
		t.Fatalf("generated config is invalid: %v", err)
	}
}

func TestWatchStartCommand(t *testing.T) {
	got, err := watchStartCommand(defaultWatchConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if want := shellArgument(os.Args[0]); got != want {
		t.Fatalf("default start command = %q, want %q", got, want)
	}
	got, err = watchStartCommand("/tmp/custom.yml")
	if err != nil {
		t.Fatal(err)
	}
	if want := shellArgument(os.Args[0]) + " --config /tmp/custom.yml"; got != want {
		t.Fatalf("custom start command = %q, want %q", got, want)
	}
	absoluteConfigFilename, err := filepath.Abs("custom.yml")
	if err != nil {
		t.Fatal(err)
	}
	got, err = watchStartCommand("custom.yml")
	if err != nil {
		t.Fatal(err)
	}
	if want := shellArgument(os.Args[0]) + " --config " + absoluteConfigFilename; got != want {
		t.Fatalf("relative custom start command = %q, want %q", got, want)
	}
	got, err = watchStartCommand("/tmp/custom config's.yml")
	if err != nil {
		t.Fatal(err)
	}
	if want := shellArgument(os.Args[0]) + " --config '/tmp/custom config'\"'\"'s.yml'"; got != want {
		t.Fatalf("quoted custom start command = %q, want %q", got, want)
	}
}

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
		if got := shellArgument(test.argument); got != test.want {
			t.Errorf("shellArgument(%q) = %q, want %q", test.argument, got, test.want)
		}
	}
}

func TestWatchAutostartTemplateIncludesExecutableAndConfig(t *testing.T) {
	entry, err := setup.RenderDesktopEntryTemplate("salmon-watch.desktop.tpl", string(mustEmbeddedAsset("assets/setup/salmon-watch.desktop.tpl")), struct {
		Executable     string
		ConfigFilename string
	}{"/home/user/.local/bin/salmon-watch", "/home/user/.config/salmon-watch/salmon-watch.yml"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Type=Application", "Exec=\"/home/user/.local/bin/salmon-watch\" --config \"/home/user/.config/salmon-watch/salmon-watch.yml\"", "Terminal=false"} {
		if !strings.Contains(entry, want) {
			t.Fatalf("entry %q does not contain %q", entry, want)
		}
	}
}

func TestWatchAutostartTemplateEscapesDesktopFieldCodes(t *testing.T) {
	entry, err := setup.RenderDesktopEntryTemplate("salmon-watch.desktop.tpl", string(mustEmbeddedAsset("assets/setup/salmon-watch.desktop.tpl")), struct {
		Executable     string
		ConfigFilename string
	}{"/tmp/sal%mon", "/tmp/a%b.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Exec=\"/tmp/sal%%mon\" --config \"/tmp/a%%b.yml\""; !strings.Contains(entry, want) {
		t.Fatalf("entry %q does not contain %q", entry, want)
	}
}
