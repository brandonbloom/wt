package timefmt

import (
	"testing"
	"time"
)

func TestRelative(t *testing.T) {
	loc := time.FixedZone("Test", -8*3600)
	ref := time.Date(2025, time.December, 5, 15, 0, 0, 0, loc)

	cases := []struct {
		name string
		ts   time.Time
		want string
	}{
		{"seconds", ref.Add(-3 * time.Second), "3s ago"},
		{"minutes", ref.Add(-2 * time.Minute), "2 min ago"},
		{"todayClock", ref.Add(-3 * time.Hour), "today 12:00pm"},
		{"yesterday", ref.Add(-26 * time.Hour), "yesterday 1:00pm"},
		{"days", ref.Add(-4 * 24 * time.Hour), "4 days ago"},
		{"sameYear", ref.AddDate(0, -2, 0), "Oct 5"},
		{"differentYear", time.Date(2023, time.January, 2, 15, 0, 0, 0, loc), "Jan 2 2023"},
		{"future", ref.Add(10 * time.Second), "just now"},
		{"unknown", time.Time{}, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Relative(tc.ts, ref); got != tc.want {
				t.Fatalf("Relative(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
