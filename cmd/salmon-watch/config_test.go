package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigNotFound(t *testing.T) {
	_, err := loadConfig(filepath.Join(t.TempDir(), "missing.yml"))
	if err == nil {
		t.Fatal("loadConfig() succeeded for a missing config")
	}
	if !configNotFound(err) {
		t.Fatalf("configNotFound(%v) = false, want true", err)
	}
}

func TestWatchConfigReadErrorOnlySuggestsSetupForDefaultConfig(t *testing.T) {
	if got := watchConfigReadError("/custom/config.yml", os.ErrNotExist); strings.Contains(got.Error(), "setup") {
		t.Fatalf("custom config error = %q, unexpectedly contains setup guidance", got)
	}
	if got := watchConfigReadError(defaultWatchConfigPath(), os.ErrNotExist); !strings.Contains(got.Error(), "setup") {
		t.Fatalf("default config error = %q, missing setup guidance", got)
	}
}

func TestLoadWatchConfigRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "salmon-watch.yml")
	if err := os.WriteFile(path, []byte("wsClient:\n  serverz: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil || !strings.Contains(err.Error(), "serverz") {
		t.Fatalf("loadConfig error = %v, want unknown-field error", err)
	}
}

func TestLoadWatchConfigRejectsInvalidServerIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "salmon-watch.yml")
	data := "wsClient:\n  servers:\n    - id: remote\n      addr: first:41990\n    - id: remote\n      addr: second:41990\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil || !strings.Contains(err.Error(), `id "remote" duplicates`) {
		t.Fatalf("loadConfig error = %v, want duplicate-ID error", err)
	}
}
