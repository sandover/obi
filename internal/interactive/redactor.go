package interactive

import "strings"

// Redactor scrubs sensitive data from transcripts/tees.
type Redactor interface {
	Redact(string) string
}

// RedactorFunc adapts a function to the Redactor interface.
type RedactorFunc func(string) string

// Redact implements Redactor.
func (f RedactorFunc) Redact(input string) string {
	if f == nil {
		return input
	}
	return f(input)
}

type secretRedactor struct {
	secrets []string
}

func newSecretRedactor(secrets []string) Redactor {
	var clean []string
	for _, secret := range secrets {
		if strings.TrimSpace(secret) == "" {
			continue
		}
		clean = append(clean, secret)
	}
	if len(clean) == 0 {
		return RedactorFunc(func(s string) string { return s })
	}
	return secretRedactor{secrets: append([]string(nil), clean...)}
}

func (s secretRedactor) Redact(input string) string {
	out := input
	for _, secret := range s.secrets {
		out = strings.ReplaceAll(out, secret, "[REDACTED]")
	}
	return out
}
