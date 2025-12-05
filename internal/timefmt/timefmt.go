package timefmt

import (
	"fmt"
	"time"
)

// Relative returns a friendly string describing how long ago t occurred
// relative to reference. If reference is zero, time.Now() is used.
func Relative(t, reference time.Time) string {
	if reference.IsZero() {
		reference = time.Now()
	}
	if t.IsZero() {
		return "unknown"
	}
	t = t.In(reference.Location())
	if t.After(reference) {
		return "just now"
	}

	diff := reference.Sub(t)
	if diff < time.Minute {
		seconds := int(diff.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		return fmt.Sprintf("%ds ago", seconds)
	}
	if diff < time.Hour {
		minutes := int(diff.Minutes())
		if minutes < 1 {
			minutes = 1
		}
		if minutes == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", minutes)
	}
	if sameDay(t, reference) {
		return fmt.Sprintf("today %s", t.Format("3:04pm"))
	}
	if isYesterday(t, reference) {
		return fmt.Sprintf("yesterday %s", t.Format("3:04pm"))
	}
	days := int(diff.Hours() / 24)
	if days < 7 {
		if days <= 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
	if t.Year() == reference.Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2 2006")
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func isYesterday(t, reference time.Time) bool {
	return sameDay(t, reference.AddDate(0, 0, -1))
}
