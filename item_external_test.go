package salmon_test

import (
	"encoding/json"
	"strings"
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

func TestItemJSONUsesDetails(t *testing.T) {
	data, err := json.Marshal(salmon.Item{Key: "probe", State: salmon.ItemStateError, Details: "command failed"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"details":"command failed"`) {
		t.Fatalf("item JSON = %s, want details field", data)
	}
	if strings.Contains(string(data), `"comment"`) {
		t.Fatalf("item JSON = %s, contains obsolete comment field", data)
	}
}
