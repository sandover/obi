package interactive

import (
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/fenced"
)

// Sentinel watches session log events and detects fenced Obi reports.
type Sentinel struct {
	parser *fenced.Parser
	result fenced.Result
	done   bool
	err    error
}

// NewSentinel constructs a sentinel bound to the provided session UUID.
func NewSentinel(sessionID string) *Sentinel {
	return &Sentinel{parser: fenced.NewParser(sessionID)}
}

// Consume ingests a session event, returning the result as soon as the fence closes.
func (s *Sentinel) Consume(evt SessionEvent) (fenced.Result, bool, error) {
	if s == nil || s.parser == nil || s.done {
		return s.result, s.done, s.err
	}
	if evt.Type != EventLogChunk {
		return fenced.Result{}, false, nil
	}
	res, done, err := s.parser.Feed(evt.Chunk)
	if err != nil {
		s.err = err
		s.done = true
		return fenced.Result{}, false, err
	}
	if done {
		s.result = res
		s.done = true
		return res, true, nil
	}
	return fenced.Result{}, false, nil
}

// Finalize flushes buffered content when the Codex stream ends.
func (s *Sentinel) Finalize() (fenced.Result, bool, error) {
	if s == nil || s.parser == nil {
		return fenced.Result{}, false, nil
	}
	if s.done {
		return s.result, true, s.err
	}
	res, done, err := s.parser.Finalize()
	if err != nil {
		s.err = err
		s.done = true
		return fenced.Result{}, false, err
	}
	if done {
		s.result = res
		s.done = true
		return res, true, nil
	}
	return fenced.Result{}, false, nil
}
