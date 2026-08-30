package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dimonomid/salmon/internal/setup"
	"github.com/dimonomid/salmon/logs"
)

func TestConfigInitCreatesConfigWithoutOverwritingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "etc", "salmon.yml")
	command := newRootCommand()
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"setup", "create-config", "--config", path})
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
	command.SetArgs([]string{"setup", "create-config", "--config", path})
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
		{"setup", "unexpected"},
		{"setup", "create-config", "unexpected"},
		{"setup", "create-user", "unexpected"},
		{"setup", "install-service", "unexpected"},
	} {
		command := newRootCommand()
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Errorf("command %q accepted an unexpected positional argument", args)
		}
	}
}

func TestSetupOperationsDoNotPolluteTopLevelCommands(t *testing.T) {
	command := newRootCommand()
	commands := command.Commands()
	if len(commands) != 1 || commands[0].Name() != "setup" {
		t.Fatalf("top-level commands = %v, want only setup", commands)
	}
	if !strings.Contains(commands[0].Long, "Perform the complete setup") {
		t.Fatalf("setup long help = %q, want complete-setup description", commands[0].Long)
	}

	setupCommands := map[string]bool{}
	for _, subcommand := range commands[0].Commands() {
		setupCommands[subcommand.Name()] = true
	}
	for _, want := range []string{"create-config", "create-user", "install-service"} {
		if !setupCommands[want] {
			t.Errorf("setup subcommands = %v, missing %q", setupCommands, want)
		}
	}
}

func TestCreateSalmonUserInstallsSysusersConfigurationAndCreatesAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sysusers.d", "salmon.conf")
	output := &bytes.Buffer{}
	var calls [][]string
	run := func(name string, args ...string) error {
		if !strings.Contains(output.String(), "Created systemd sysusers configuration") {
			t.Fatal("sysusers command ran before file creation was reported")
		}
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	if err := createSalmonUserAt(output, path, run); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), string(mustSetupAsset("assets/setup/salmon.sysusers")); got != want {
		t.Fatalf("sysusers configuration = %q, want %q", got, want)
	}
	if got, want := len(calls), 1; got != want {
		t.Fatalf("command count = %d, want %d", got, want)
	}
	if got, want := strings.Join(calls[0], " "), "systemd-sysusers "+path; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if !strings.Contains(output.String(), "Created systemd sysusers configuration") {
		t.Fatalf("unexpected command output: %q", output.String())
	}
}

func TestRequireSalmonServiceAccountChecksUserAndGroup(t *testing.T) {
	lookupUser := func(name string) (*user.User, error) {
		if name != salmonUserName {
			t.Fatalf("user name = %q, want %q", name, salmonUserName)
		}
		return &user.User{Username: name}, nil
	}
	lookupGroup := func(name string) (*user.Group, error) {
		if name != salmonGroupName {
			t.Fatalf("group name = %q, want %q", name, salmonGroupName)
		}
		return &user.Group{Name: name}, nil
	}
	if err := requireSalmonServiceAccountWith(lookupUser, lookupGroup); err != nil {
		t.Fatal(err)
	}

	err := requireSalmonServiceAccountWith(
		func(string) (*user.User, error) { return nil, user.UnknownUserError(salmonUserName) },
		lookupGroup,
	)
	if err == nil || !strings.Contains(err.Error(), "sudo salmon setup create-user") {
		t.Fatalf("missing-user error = %v, want user-create guidance", err)
	}

	err = requireSalmonServiceAccountWith(
		lookupUser,
		func(string) (*user.Group, error) { return nil, user.UnknownGroupError(salmonGroupName) },
	)
	if err == nil || !strings.Contains(err.Error(), "sudo salmon setup create-user") {
		t.Fatalf("missing-group error = %v, want user-create guidance", err)
	}
}

func TestLogLevelFlagDefaultsToInfoAndRejectsInvalidValues(t *testing.T) {
	command := newRootCommand()
	flag := command.Flags().Lookup("log-level")
	if flag == nil || flag.DefValue != "info" {
		t.Fatalf("log-level flag = %#v, want default info", flag)
	}
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--log-level", "verbose"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("error = %v, want invalid-log-level error", err)
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
	for _, want := range []string{"User=salmon", "Group=salmon", "ExecStart=\"/usr/local/bin/salmon\" --config \"/etc/salmon.yml\"", "WantedBy=multi-user.target"} {
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
	err := runSalmon(path, logs.Info)
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

func TestLoadConfigUsesWebserverTLSOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "salmon.yml")
	data := []byte(`core:
  messengers:
    - webserver:
        listenAddress: 127.0.0.1:41990
        tls:
          certFile: /etc/salmon/tls/fullchain.pem
          keyFile: /etc/salmon/tls/privkey.pem
        auth:
          - id: my-laptop
            bearerTokenHash: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig := cfg.Core.Messengers[0].Webserver.TLS
	if tlsConfig == nil || tlsConfig.CertFile != "/etc/salmon/tls/fullchain.pem" || tlsConfig.KeyFile != "/etc/salmon/tls/privkey.pem" {
		t.Fatalf("TLS config = %#v", tlsConfig)
	}
	auth := cfg.Core.Messengers[0].Webserver.Auth
	if len(auth) != 1 || auth[0].ID != "my-laptop" || auth[0].BearerTokenHash == "" {
		t.Fatalf("auth config = %#v", auth)
	}
}

func TestLoadConfigRejectsOldNestedBearerTokenAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "salmon.yml")
	data := []byte(`core:
  messengers:
    - webserver:
        listenAddress: 127.0.0.1:41990
        auth:
          bearerTokens:
            - id: my-laptop
              tokenHash: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("old nested bearer-token authentication was accepted")
	}
}
