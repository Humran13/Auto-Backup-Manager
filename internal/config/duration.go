package config

import "time"

// ParseDuration parses retention window strings. Go's time.ParseDuration has
// no unit for days, so callers write the project default as "240h" (10 days)
// rather than "10d"; this wrapper exists as the single place that decision
// would change if we ever add a "d" unit.
func ParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}
