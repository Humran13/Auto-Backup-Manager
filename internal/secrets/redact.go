// Package secrets stores and retrieves sensitive values (restic repository
// passwords, database credentials) using OS-native protection, and provides
// a redaction helper used everywhere diagnostics or logs might otherwise
// leak a secret.
package secrets

import "regexp"

// patterns matches common secret-bearing key=value and URL-embedded-password
// shapes so they can be masked before text reaches a log line or `abm doctor`
// output. It is deliberately broad: false positives (over-redaction) are
// harmless, false negatives (a leaked secret) are not.
var patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|access_key|secret_key|api_key|client_secret)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)://[^:/@\s]+:[^@/\s]+@`), // user:password@host in a URL
}

// Redact masks any substring of s that matches a known secret shape.
func Redact(s string) string {
	out := s
	for _, p := range patterns {
		out = p.ReplaceAllStringFunc(out, func(match string) string {
			if kv := keyOnly.FindStringSubmatch(match); kv != nil {
				return kv[1] + "=[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return out
}

var keyOnly = regexp.MustCompile(`(?i)^([a-z_]+)\s*[:=]`)
