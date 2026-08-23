package salmon_test

import (
	"testing"

	"github.com/dimonomid/salmon"
)

func TestIsItemStateValid(t *testing.T) {
	for _, state := range []salmon.ItemState{
		salmon.ItemStateOK,
		salmon.ItemStateWarning,
		salmon.ItemStateError,
	} {
		if !salmon.IsItemStateValid(state) {
			t.Errorf("IsItemStateValid(%q) = false, want true", state)
		}
	}

	for _, state := range []salmon.ItemState{"", "unknown", "OK"} {
		if salmon.IsItemStateValid(state) {
			t.Errorf("IsItemStateValid(%q) = true, want false", state)
		}
	}
}
