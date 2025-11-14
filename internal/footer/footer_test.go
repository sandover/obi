package footer

import "testing"

func TestParseSuccess(t *testing.T) {
	out := `something
STATUS: success
COMMIT_MSG:
fix stuff`
	res, err := Parse(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Status != StatusSuccess || res.CommitMsg != "fix stuff" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestParseNeedsEscalation(t *testing.T) {
	out := `STATUS: needs_help
COMMIT_MSG:
wip
ESCALATION: wait for approval`
	res, err := Parse(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Escalation == "" {
		t.Fatalf("expected escalation text")
	}
}

func TestParseMissingFields(t *testing.T) {
	_, err := Parse("STATUS: success")
	if err == nil {
		t.Fatalf("expected error")
	}
}
