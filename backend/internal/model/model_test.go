package model

import (
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	cases := []struct {
		in   string
		want Window
		ok   bool
	}{
		{"day", WindowDay, true},
		{"week", WindowWeek, true},
		{"month", WindowMonth, true},
		{"year", "", false},
		{"", "", false},
		{"DAY", "", false},
	}
	for _, c := range cases {
		got, err := ParseWindow(c.in)
		if c.ok && err != nil {
			t.Errorf("ParseWindow(%q) unexpected error: %v", c.in, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ParseWindow(%q) expected error, got nil", c.in)
		}
		if c.ok && got != c.want {
			t.Errorf("ParseWindow(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestWindowDuration(t *testing.T) {
	if d := WindowDay.Durations(); d != 24*time.Hour {
		t.Errorf("day = %v, want 24h", d)
	}
	if d := WindowWeek.Durations(); d != 7*24*time.Hour {
		t.Errorf("week = %v, want 168h", d)
	}
	if d := WindowMonth.Durations(); d != 30*24*time.Hour {
		t.Errorf("month = %v, want 720h", d)
	}
}

func TestAllWindows(t *testing.T) {
	ws := AllWindows()
	if len(ws) != 3 {
		t.Fatalf("expected 3 windows, got %d", len(ws))
	}
}
