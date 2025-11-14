package fenced

import (
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
)

func TestParserParsesFencedReport(t *testing.T) {
	parser := NewParser("session-42")
	chunks := []string{
		"booting...\nnoise\n",
		"```obi:session-42\nstatus: success\ncommit_msg: Ship feature\n",
		"details: |\n  Added foo\n  Step 2: verify\n",
		"```\n",
	}

	var (
		res  Result
		done bool
		err  error
	)

	for _, chunk := range chunks {
		res, done, err = parser.Feed(chunk)
		if err != nil {
			t.Fatalf("feed error: %v", err)
		}
		if done {
			break
		}
	}

	if !done {
		res, done, err = parser.Finalize()
		if err != nil {
			t.Fatalf("finalize: %v", err)
		}
	}

	if res.SessionID != "session-42" {
		t.Fatalf("unexpected session id %q", res.SessionID)
	}
	if res.Status != "success" {
		t.Fatalf("unexpected status %q", res.Status)
	}
	if res.CommitMsg != "Ship feature" {
		t.Fatalf("unexpected commit msg %q", res.CommitMsg)
	}
	expectedDetails := "Added foo\nStep 2: verify"
	if res.Details != expectedDetails {
		t.Fatalf("details mismatch: %q", res.Details)
	}
}

func TestParserRequiresMatchingSessionID(t *testing.T) {
	parser := NewParser("session-1")
	_, _, err := parser.Feed("```obi:other\n")
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
}

func TestParserRequiresClosingFence(t *testing.T) {
	parser := NewParser("abc")
	if _, _, err := parser.Feed("```obi:abc\nstatus: success\ncommit_msg: done\ndetails: |\n  hi\n"); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if _, _, err := parser.Finalize(); err == nil {
		t.Fatalf("expected closing fence error")
	}
}

func TestParserNeedsEscalationWhenFailure(t *testing.T) {
	parser := NewParser("abc")
	chunk := "```obi:abc\nstatus: needs_help\ncommit_msg: blocked\ndetails: |\n  waiting\n```\n"
	if _, _, err := parser.Feed(chunk); err == nil {
		t.Fatalf("expected escalation error")
	}
}

func TestParserHandlesInlineDetails(t *testing.T) {
	parser := NewParser("abc")
	chunk := "```obi:abc\nstatus: success\ncommit_msg: hey\ndetails: All done\n```\n"
	res, done, err := parser.Feed(chunk)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if !done {
		t.Fatalf("expected done")
	}
	if res.Details != "All done" {
		t.Fatalf("unexpected details %q", res.Details)
	}
}

func TestParserHandlesStreamingChunksWithNoise(t *testing.T) {
	parser := NewParser("session-100")
	chunks := []string{
		"\nrandom\nPREFIX\n```obi:session",
		"-100\nstatus:",
		" success\ncommit_msg: Chunked input\ndetails: |\n",
		"  first line\n  second line\n",
		"```\n",
	}
	var (
		res  Result
		done bool
		err  error
	)
	for _, chunk := range chunks {
		res, done, err = parser.Feed(chunk)
		if err != nil {
			t.Fatalf("feed chunk err=%v", err)
		}
		if done {
			break
		}
	}
	if !done {
		res, done, err = parser.Finalize()
		if err != nil {
			t.Fatalf("finalize: %v", err)
		}
		if !done {
			t.Fatalf("expected parser to finish")
		}
	}
	if res.Details != "first line\nsecond line" {
		t.Fatalf("details mismatch: %q", res.Details)
	}
}

func TestParserNeedsHelpSuccessPath(t *testing.T) {
	parser := NewParser("needs-help")
	chunk := "```obi:needs-help\nstatus: needs_help\ncommit_msg: Blocked\ndetails: |\n  waiting\nescalation: approval needed\n```\n"
	res, done, err := parser.Feed(chunk)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if !done {
		t.Fatalf("expected parser to finish")
	}
	if res.Status != footer.StatusFailure {
		t.Fatalf("expected needs_help status, got %s", res.Status)
	}
	if res.Escalation != "approval needed" {
		t.Fatalf("unexpected escalation %q", res.Escalation)
	}
}

func TestParserErrorsWhenDuplicateFenceAppears(t *testing.T) {
	parser := NewParser("dup")
	chunk := "```obi:dup\nstatus: success\ncommit_msg: start\ndetails: |\n  hi\n```obi:dup\n"
	if _, _, err := parser.Feed(chunk); err != nil {
		t.Fatalf("feed: %v", err)
	}
	if _, _, err := parser.Finalize(); err == nil {
		t.Fatalf("expected finalize error due to duplicate fence")
	}
}
