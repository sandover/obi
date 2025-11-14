package interactive

import (
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/fenced"
)

func TestSentinelEmitsResult(t *testing.T) {
	sentinel := NewSentinel("session-x")
	events := []SessionEvent{
		{Type: EventLogChunk, Chunk: "booting\n```obi:session-x\n"},
		{Type: EventLogChunk, Chunk: "status: success\ncommit_msg: shipped\n"},
		{Type: EventLogChunk, Chunk: "details: |\n  done\n```\n"},
	}

	var (
		res  fenced.Result
		done bool
		err  error
	)

	for _, evt := range events {
		res, done, err = sentinel.Consume(evt)
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if done {
			break
		}
	}

	if !done {
		res, done, err = sentinel.Finalize()
		if err != nil {
			t.Fatalf("finalize: %v", err)
		}
	}

	if res.Status != "success" {
		t.Fatalf("unexpected status %q", res.Status)
	}
	if res.Details != "done" {
		t.Fatalf("unexpected details %q", res.Details)
	}
}

func TestSentinelPropagatesErrors(t *testing.T) {
	sentinel := NewSentinel("session-x")
	_, _, err := sentinel.Consume(SessionEvent{Type: EventLogChunk, Chunk: "```obi:wrong\n"})
	if err == nil {
		t.Fatalf("expected error from mismatch session id")
	}
}

func TestSentinelFinalizeMissingFence(t *testing.T) {
	sentinel := NewSentinel("session-x")
	if _, _, err := sentinel.Finalize(); err == nil {
		t.Fatalf("expected missing fence error")
	}
}
