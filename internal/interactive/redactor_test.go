package interactive

import (
	"strings"
	"testing"
)

func TestSecretRedactorScrubsMultiWordAndUnicode(t *testing.T) {
	r := newSecretRedactor([]string{"token", "multi word", "秘密"})
	input := "token + multi word + 秘密 should vanish"
	if got := r.Redact(input); strings.Count(got, "[REDACTED]") != 3 {
		t.Fatalf("expected every secret to be redacted, got %q", got)
	}
}

func TestSecretRedactorHandlesOverlappingSecrets(t *testing.T) {
	r := newSecretRedactor([]string{"abc", "abcd"})
	got := r.Redact("abcd abc")
	if strings.Count(got, "[REDACTED]") != 2 {
		t.Fatalf("expected both overlapping tokens to be redacted, got %q", got)
	}
}
