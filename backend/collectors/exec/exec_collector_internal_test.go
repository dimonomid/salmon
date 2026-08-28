package exec

import (
	"strings"
	"testing"
	"time"
)

func TestApplyConfigDefaultsChoosesShortestTimeout(t *testing.T) {
	tests := []struct {
		name          string
		config        Config
		wantTimeout   time.Duration
		wantPoll      time.Duration
		wantUnhealthy time.Duration
	}{
		{
			name:          "all defaults",
			wantTimeout:   5 * time.Second,
			wantPoll:      time.Minute,
			wantUnhealthy: 5 * time.Second,
		},
		{
			name:          "normal interval is shortest",
			config:        Config{PollInterval: 2 * time.Second, PollIntervalWhenUnhealthy: 10 * time.Second},
			wantTimeout:   2 * time.Second,
			wantPoll:      2 * time.Second,
			wantUnhealthy: 10 * time.Second,
		},
		{
			name:          "unhealthy interval is shortest",
			config:        Config{PollInterval: 10 * time.Second, PollIntervalWhenUnhealthy: 2 * time.Second},
			wantTimeout:   2 * time.Second,
			wantPoll:      10 * time.Second,
			wantUnhealthy: 2 * time.Second,
		},
		{
			name:          "one minute ceiling is shortest",
			config:        Config{PollInterval: 2 * time.Minute, PollIntervalWhenUnhealthy: 3 * time.Minute},
			wantTimeout:   time.Minute,
			wantPoll:      2 * time.Minute,
			wantUnhealthy: 3 * time.Minute,
		},
		{
			name: "explicit timeout is retained",
			config: Config{
				PollInterval:              2 * time.Minute,
				PollIntervalWhenUnhealthy: 3 * time.Minute,
				Timeout:                   90 * time.Second,
			},
			wantTimeout:   90 * time.Second,
			wantPoll:      2 * time.Minute,
			wantUnhealthy: 3 * time.Minute,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := test.config
			applyConfigDefaults(&config)
			if config.Timeout != test.wantTimeout ||
				config.PollInterval != test.wantPoll ||
				config.PollIntervalWhenUnhealthy != test.wantUnhealthy {
				t.Fatalf("defaults = %#v, want timeout %s, poll %s, unhealthy poll %s",
					config, test.wantTimeout, test.wantPoll, test.wantUnhealthy)
			}
		})
	}
}

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
