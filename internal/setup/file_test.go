package setup

import (
	"io/ioutil"
	"path/filepath"
	"testing"
)

func TestEnsureFileDoesNotOverwriteExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "etc", "salmon.yml")
	created, err := EnsureFile(path, "first\n")
	if err != nil || !created {
		t.Fatalf("EnsureFile() = (%v, %v), want (true, nil)", created, err)
	}

	created, err = EnsureFile(path, "second\n")
	if err != nil || created {
		t.Fatalf("EnsureFile() = (%v, %v), want (false, nil)", created, err)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "first\n"; got != want {
		t.Fatalf("config contents = %q, want %q", got, want)
	}
	entries, err := ioutil.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "salmon.yml" {
		t.Fatalf("directory entries = %#v, want only salmon.yml", entries)
	}
}
