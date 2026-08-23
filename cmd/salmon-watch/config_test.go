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
