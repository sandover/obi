package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"unicode"
)

const softStopReasonDefault = "Operator requested soft stop (hotkey 's')"

// SessionControls exposes the session operations needed for input routing.
type SessionControls interface {
	WriteInput([]byte) (int, error)
	SoftStop(reason string) error
	Abort() error
}

// HintSubmitter receives finalized hint text.
type HintSubmitter interface {
	SubmitHint(text string) error
}

// ShellBindings capture the shell operations the router depends on.
type ShellBindings interface {
	TogglePause() bool
	SetHintInput(active bool, text string)
	ToggleHelp() bool
}

// InputMode identifies the current routing mode.
type InputMode int

const (
	// ModePassthrough sends keys directly to Codex.
	ModePassthrough InputMode = iota
	// ModeHint captures characters for the inline hint entry UI.
	ModeHint
)

// InputRouter interprets keystrokes, triggering hotkeys or forwarding bytes.
type InputRouter struct {
	session         SessionControls
	shell           ShellBindings
	hints           HintSubmitter
	mode            InputMode
	hintBuf         []rune
	softStopReason  string
	cancelSequences map[byte]struct{}
}

// InputOption customizes router behavior.
type InputOption func(*InputRouter)

// WithHintSubmitter registers a callback for finalized hints.
func WithHintSubmitter(sub HintSubmitter) InputOption {
	return func(r *InputRouter) {
		r.hints = sub
	}
}

// WithSoftStopReason overrides the default reason passed to Codex.
func WithSoftStopReason(reason string) InputOption {
	return func(r *InputRouter) {
		if strings.TrimSpace(reason) == "" {
			return
		}
		r.softStopReason = reason
	}
}

// NewInputRouter wires keyboard input to the session and shell bindings.
func NewInputRouter(session SessionControls, shell ShellBindings, opts ...InputOption) *InputRouter {
	router := &InputRouter{
		session:        session,
		shell:          shell,
		mode:           ModePassthrough,
		softStopReason: softStopReasonDefault,
		cancelSequences: map[byte]struct{}{
			0x1b: {}, // ESC
		},
	}
	for _, opt := range opts {
		opt(router)
	}
	return router
}

// Run consumes bytes from r until ctx is cancelled or EOF is reached.
func (r *InputRouter) Run(ctx context.Context, src io.Reader) error {
	if src == nil {
		return errors.New("input source is required")
	}
	buf := make([]byte, 64)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := src.Read(buf)
		if n > 0 {
			if handleErr := r.HandleBytes(buf[:n]); handleErr != nil {
				return handleErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// HandleBytes applies routing logic to the provided bytes.
func (r *InputRouter) HandleBytes(data []byte) error {
	for _, b := range data {
		if err := r.handleByte(b); err != nil {
			return err
		}
	}
	return nil
}

// Mode reports the current routing mode.
func (r *InputRouter) Mode() InputMode {
	return r.mode
}

// HintText exposes the in-progress hint contents.
func (r *InputRouter) HintText() string {
	return string(r.hintBuf)
}

func (r *InputRouter) handleByte(b byte) error {
	switch r.mode {
	case ModeHint:
		return r.handleHintByte(b)
	default:
		return r.handlePassthroughByte(b)
	}
}

func (r *InputRouter) handlePassthroughByte(b byte) error {
	switch unicode.ToLower(rune(b)) {
	case 'p':
		if r.shell != nil {
			r.shell.TogglePause()
		}
		return nil
	case 'h':
		r.startHintCapture()
		return nil
	case 's':
		if r.session == nil {
			return errors.New("session controls unavailable for soft stop")
		}
		reason := r.softStopReason
		if strings.TrimSpace(reason) == "" {
			reason = softStopReasonDefault
		}
		return r.session.SoftStop(reason)
	case 'q':
		if r.session == nil {
			return errors.New("session controls unavailable for abort")
		}
		return r.session.Abort()
	}
	if b == '?' {
		if r.shell != nil {
			r.shell.ToggleHelp()
		}
		return nil
	}
	if r.session == nil {
		return errors.New("session controls unavailable for pass-through input")
	}
	_, err := r.session.WriteInput([]byte{b})
	return err
}

func (r *InputRouter) handleHintByte(b byte) error {
	if _, ok := r.cancelSequences[b]; ok {
		r.exitHintCapture()
		return nil
	}
	switch b {
	case '\r', '\n':
		return r.finalizeHint()
	case 0x7f, 0x08:
		if len(r.hintBuf) > 0 {
			r.hintBuf = r.hintBuf[:len(r.hintBuf)-1]
			r.syncHintUI()
		}
		return nil
	default:
		r.hintBuf = append(r.hintBuf, rune(b))
		r.syncHintUI()
		return nil
	}
}

func (r *InputRouter) startHintCapture() {
	r.mode = ModeHint
	r.hintBuf = r.hintBuf[:0]
	r.syncHintUI()
}

func (r *InputRouter) exitHintCapture() {
	r.mode = ModePassthrough
	r.hintBuf = r.hintBuf[:0]
	if r.shell != nil {
		r.shell.SetHintInput(false, "")
	}
}

func (r *InputRouter) finalizeHint() error {
	text := string(r.hintBuf)
	if strings.TrimSpace(text) != "" && r.hints != nil {
		if err := r.hints.SubmitHint(text); err != nil {
			return err
		}
	}
	r.exitHintCapture()
	return nil
}

func (r *InputRouter) syncHintUI() {
	if r.shell == nil {
		return
	}
	r.shell.SetHintInput(true, string(r.hintBuf))
}
