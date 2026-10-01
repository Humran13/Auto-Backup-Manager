package secrets

import (
	"strings"
	"testing"
)

func TestRedact_KeyValuePairs(t *testing.T) {
	cases := []string{
		"password=hunter2",
		"password: hunter2",
		"DB_PASSWORD=SuperSecret123",
		"secret_key=AKIAABCDEFGHIJKLMNOP",
		"client_secret: abc123XYZ",
		"api_key=sk-1234567890",
	}
	for _, in := range cases {
		out := Redact(in)
		if strings.Contains(out, "hunter2") || strings.Contains(out, "SuperSecret123") ||
			strings.Contains(out, "AKIAABCDEFGHIJKLMNOP") || strings.Contains(out, "abc123XYZ") ||
			strings.Contains(out, "sk-1234567890") {
			t.Fatalf("Redact(%q) = %q, secret leaked", in, out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Fatalf("Redact(%q) = %q, expected a redaction marker", in, out)
		}
	}
}

func TestRedact_URLEmbeddedPassword(t *testing.T) {
	in := "connecting to postgres://dbuser:s3cr3tPass@db.internal:5432/app"
	out := Redact(in)
	if strings.Contains(out, "s3cr3tPass") {
		t.Fatalf("Redact(%q) = %q, password leaked", in, out)
	}
}

func TestRedact_LeavesNonSecretTextAlone(t *testing.T) {
	in := "backup completed: 42 files new, 3 changed"
	if out := Redact(in); out != in {
		t.Fatalf("Redact modified non-secret text: got %q, want %q", out, in)
	}
}
