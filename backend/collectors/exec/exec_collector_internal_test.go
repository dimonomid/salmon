package exec

import (
	"strings"
	"testing"
)

func TestFirstLineWriter(t *testing.T) {
	var writer firstLineWriter
	for _, text := range []string{"first", " line\nsecond", "ignored"} {
		if _, err := writer.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := writer.String(), "first line"; got != want {
		t.Fatalf("first line = %q, want %q", got, want)
	}
}

func TestFirstLineWriterTruncatesLongLines(t *testing.T) {
	var writer firstLineWriter
	input := strings.Repeat("x", maxOutputLineBytes+1)
	if _, err := writer.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	if got, want := writer.String(), strings.Repeat("x", maxOutputLineBytes)+"…"; got != want {
		t.Fatalf("truncated line length = %d, want %d", len(got), len(want))
	}
}
