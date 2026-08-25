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

func TestConfigInitCreatesConfigWithoutOverwritingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "etc", "salmon.yml")
	command := newRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"config", "init", "--config", path})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Created configuration") {
		t.Fatalf("unexpected command output: %q", output.String())
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), string(mustSetupAsset("assets/setup/salmon.yml")); got != want {
		t.Fatalf("config contents = %q, want %q", got, want)
	}
	if _, err := loadConfig(path); err != nil {
		t.Fatalf("generated config is invalid: %v", err)
	}

	command = newRootCommand()
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"config", "init", "--config", path})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err = ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), string(mustSetupAsset("assets/setup/salmon.yml")); got != want {
		t.Fatalf("config was overwritten: got %q, want %q", got, want)
	}
}

func TestRunnableCommandsRejectPositionalArguments(t *testing.T) {
	for _, args := range [][]string{
		{"unexpected"},
		{"config", "init", "unexpected"},
		{"service", "install", "unexpected"},
		{"setup", "unexpected"},
	} {
		command := newRootCommand()
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Errorf("command %q accepted an unexpected positional argument", args)
		}
	}
}

func TestSalmonServiceTemplateIncludesExecutableAndConfig(t *testing.T) {
	unit, err := setup.RenderSystemdUnitTemplate("salmon.service.tpl", string(mustSetupAsset("assets/setup/salmon.service.tpl")), struct {
		Executable     string
		ConfigFilename string
	}{"/usr/local/bin/salmon", "/etc/salmon.yml"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ExecStart=\"/usr/local/bin/salmon\" --config \"/etc/salmon.yml\"", "WantedBy=multi-user.target"} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit %q does not contain %q", unit, want)
		}
	}
}

func TestSalmonServiceTemplateEscapesSystemdSpecifiers(t *testing.T) {
	unit, err := setup.RenderSystemdUnitTemplate("salmon.service.tpl", string(mustSetupAsset("assets/setup/salmon.service.tpl")), struct {
		Executable     string
		ConfigFilename string
	}{"/tmp/sal%mon", "/tmp/a%b.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "ExecStart=\"/tmp/sal%%mon\" --config \"/tmp/a%%b.yml\""; !strings.Contains(unit, want) {
		t.Fatalf("unit %q does not contain %q", unit, want)
	}
}

func TestRunSalmonSuggestsSetupWhenConfigIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yml")
	err := runSalmon(path)
	if err == nil || strings.Contains(err.Error(), "setup") {
		t.Fatalf("runSalmon() error = %v, want no setup guidance for custom config", err)
	}
}

func TestSalmonConfigReadErrorSuggestsSetupForDefaultConfig(t *testing.T) {
	err := salmonConfigReadError(defaultSalmonConfig, os.ErrNotExist)
	want := "Hint: Run the following command to create the default configuration and install the service:\n\n    sudo " + os.Args[0] + " setup"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("salmonConfigReadError() = %v, want setup guidance", err)
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "salmon.yml")
	if err := os.WriteFile(path, []byte("core:\n  collectorz: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil || !strings.Contains(err.Error(), "collectorz") {
		t.Fatalf("loadConfig error = %v, want unknown-field error", err)
	}
}

func TestLoadConfigUsesSystemdRuleNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "salmon.yml")
	data := []byte(`core:
  collectors:
    - id: services
      systemd:
        unitRules:
          - names: [one.service, two.service]
            conditions:
              - {result: ok}
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Core.Collectors[0].Systemd.UnitRules[0].Names
	if strings.Join(got, ",") != "one.service,two.service" {
		t.Fatalf("rule names = %#v", got)
	}
}
