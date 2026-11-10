package schedule

import (
	"testing"
	"time"
)

func TestParseInterval(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"": DefaultInterval, "6h": 6 * time.Hour, "1d": 24 * time.Hour, "2w": 14 * 24 * time.Hour, "90m": 90 * time.Minute,
	} {
		if got, err := ParseInterval(in); err != nil || got != want {
			t.Errorf("ParseInterval(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"x", "0d", "-1w", "d"} {
		if _, err := ParseInterval(bad); err == nil {
			t.Errorf("ParseInterval(%q) succeeded", bad)
		}
	}
}

func TestDescribe(t *testing.T) {
	for mins, want := range map[int]string{60: "hour", 360: "6 hours", 1440: "day", 2880: "2 days", 10080: "week", 45: "45 minutes"} {
		if got := Describe(mins); got != want {
			t.Errorf("Describe(%d) = %q, want %q", mins, got, want)
		}
	}
}
