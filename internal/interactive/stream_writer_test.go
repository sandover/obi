package interactive

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamWriterRedactsTeeButNotLive(t *testing.T) {
	var live bytes.Buffer
	var tee bytes.Buffer
	writer := newStreamWriter(&live, &tee, newSecretRedactor([]string{"SECRET"}))

	data := []byte("hello SECRET world")
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("write stream: %v", err)
	}

	if !strings.Contains(live.String(), "SECRET") {
		t.Fatalf("live output should contain raw secret, got %q", live.String())
	}
	if strings.Contains(tee.String(), "SECRET") {
		t.Fatalf("tee output should redact secret, got %q", tee.String())
	}
	if got := writer.Redacted(); strings.Contains(got, "SECRET") {
		t.Fatalf("recorded buffer should be redacted, got %q", got)
	}
}
