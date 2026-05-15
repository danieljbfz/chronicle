package composition

import (
	"fmt"
	"time"
)

// This file holds the small human-readable formatters the
// trash listing uses. They live separately from trash.go
// because they are pure presentation helpers that have
// nothing to do with the trash model itself. Keeping them
// here means the trash file can focus on the model and
// these tiny functions stay out of its way.

// humanBytes turns a byte count into a short human-readable
// string. The function picks the largest unit that gives a
// value under 1024, so 2050 becomes "2.0KB" instead of
// "2050B" and a small file like 856 bytes stays "856B"
// instead of becoming "0.8KB".
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// humanAge turns a past timestamp into a short relative
// string like "2h ago" or "3d ago". The trash listing uses
// this to give the user a sense of how much retention time
// is left without making them do date math.
func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}
