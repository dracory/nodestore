package nodestore

import (
	"strings"
	"time"

	"github.com/dromara/carbon/v2"
)

// nowUTC returns the current UTC time formatted the same way
// entitystore formats its timestamps
func nowUTC() string {
	return carbon.Now(carbon.UTC).ToDateTimeString(carbon.UTC)
}

// parseDatetime parses an entitystore-style datetime string.
// Returns false when the value is empty or unparsable.
func parseDatetime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(datetimeLayout, value)
	if err == nil {
		return t, true
	}
	// Some drivers (e.g. modernc SQLite) return DATETIME columns in
	// RFC3339 form — accept it too.
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// formatDatetime formats a time the entitystore way
func formatDatetime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(datetimeLayout)
}

// unifyName normalizes names used for lookups
func unifyName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
