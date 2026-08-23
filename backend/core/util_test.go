package core

import (
	"strings"
	"testing"
)

func TestCheckID(t *testing.T) {
	for _, id := range []string{"salmon", "salmon-watch", "salmon_2"} {
		if err := CheckID(id); err != nil {
			t.Errorf("CheckID(%q) = %v, want nil", id, err)
		}
	}

	for _, id := range []string{"", "2salmon", "salmon!"} {
		err := CheckID(id)
		if err == nil {
			t.Errorf("CheckID(%q) = nil, want error", id)
			continue
		}
		if id != "" && !strings.Contains(err.Error(), "\""+id+"\"") {
			t.Errorf("CheckID(%q) error = %q, want ID in message", id, err)
		}
	}
}
