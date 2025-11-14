package interactive

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/codexexec"
)

const (
	// SoftStopMarker is sent to Codex when operators request an orderly shutdown.
	SoftStopMarker = "[[OBI:SOFT_STOP]]"
	// HumanHintMarker precedes operator hints injected into Codex.
	HumanHintMarker = "[[OBI:HUMAN_HINT]]"

	eventBufferSize = 64
	pipeLauncherEnv = "OBI_PIPE_LAUNCHER"
)

// SessionRunner launches Codex inside a PTY and surfaces lifecycle controls.
type SessionRunner struct {
	launcher  launcher
	preflight func() error
	newUUID   func() (string, error)
	now       func() time.Time
}

// SessionRunnerOption customizes the SessionRunner (primarily for tests).
type SessionRunnerOption func(*SessionRunner)

// NewSessionRunner constructs a SessionRunner with optional overrides.
func NewSessionRunner(opts ...SessionRunnerOption) *SessionRunner {
	r := &SessionRunner{
		launcher:  defaultLauncher(),
		preflight: defaultPreflight,
		newUUID:   randomSessionUUID,
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// WithLauncher injects a fake launcher (used by tests).
func WithLauncher(l launcher) SessionRunnerOption {
	return func(r *SessionRunner) {
		r.launcher = l
	}
}

// WithPreflight overrides the PTY preflight check (used by tests).
func WithPreflight(fn func() error) SessionRunnerOption {
	return func(r *SessionRunner) {
		r.preflight = fn
	}
}

// WithUUIDGenerator overrides the session UUID generator (used by tests).
func WithUUIDGenerator(fn func() (string, error)) SessionRunnerOption {
	return func(r *SessionRunner) {
		r.newUUID = fn
	}
}

func defaultLauncher() launcher {
	if usePipeLauncher() {
		return pipeLauncher{}
	}
	return realLauncher{}
}

func usePipeLauncher() bool {
	return os.Getenv(pipeLauncherEnv) == "1"
}

// PreparedPrompt captures the final prompt text plus its session UUID.
type PreparedPrompt struct {
	SessionID string
	Text      string
}

// PreparePrompt appends fenced-report instructions to the provided body and
// returns the final prompt plus a unique session ID.
func (r *SessionRunner) PreparePrompt(body string) (PreparedPrompt, error) {
	if r == nil {
		r = NewSessionRunner()
	}
	if r.newUUID == nil {
		r.newUUID = randomSessionUUID
	}
	id, err := r.newUUID()
	if err != nil {
		return PreparedPrompt{}, fmt.Errorf("generate session id: %w", err)
	}
	body = strings.TrimSpace(body)
	instructions := fencedReportInstructions(id)
	var prompt string
	if body == "" {
		prompt = instructions
	} else {
		prompt = fmt.Sprintf("%s\n\n%s", body, instructions)
	}
	return PreparedPrompt{SessionID: id, Text: prompt}, nil
}

// StartOptions configure a Codex session launch.
type StartOptions struct {
	SessionID  string
	Prompt     string
	Invocation codexexec.Invocation
	Stdout     io.Writer
	Tee        io.Writer
	Redactor   Redactor
	Secrets    []string
	Dir        string
	Env        []string
}

// SessionHandle exposes lifecycle controls plus result waiting.
type SessionHandle struct {
	exec *sessionExecution
}

// Events streams structured events for TUI consumers. The channel closes once
// the Codex process exits.
func (h *SessionHandle) Events() <-chan SessionEvent {
	if h == nil || h.exec == nil {
		return nil
	}
	return h.exec.events
}

// SoftStop injects a marker instructing Codex to wrap up gracefully.
func (h *SessionHandle) SoftStop(reason string) error {
	if h == nil || h.exec == nil {
		return errors.New("session not running")
	}
	return h.exec.softStop(reason)
}

// Abort sends SIGINT (falling back to SIGKILL) to Codex.
func (h *SessionHandle) Abort() error {
	if h == nil || h.exec == nil {
		return errors.New("session not running")
	}
	return h.exec.abort()
}

// SubmitHint injects a human hint into the Codex PTY.
func (h *SessionHandle) SubmitHint(text string) error {
	if h == nil || h.exec == nil {
		return errors.New("session not running")
	}
	return h.exec.submitHint(text)
}

// WriteInput forwards raw bytes into the Codex PTY.
func (h *SessionHandle) WriteInput(data []byte) (int, error) {
	if h == nil || h.exec == nil {
		return 0, errors.New("session not running")
	}
	return h.exec.writeInput(data)
}

// Wait blocks until the Codex process exits and returns the session Result.
func (h *SessionHandle) Wait() (Result, error) {
	if h == nil || h.exec == nil {
		return Result{}, errors.New("session not running")
	}
	return h.exec.wait()
}

// Result captures the structured outcome of a Codex session.
type Result struct {
	SessionID   string
	Prompt      string
	Invocation  codexexec.Invocation
	Output      string
	ExitCode    int
	StartedAt   time.Time
	CompletedAt time.Time
}

// SessionEventType categorizes events surfaced by the SessionRunner.
type SessionEventType string

const (
	// EventLogChunk indicates the Chunk field holds raw PTY output.
	EventLogChunk SessionEventType = "log_chunk"
	// EventStateChange indicates the State field changed.
	EventStateChange SessionEventType = "state_change"
	// EventExit indicates Codex exited; ExitCode/Error are populated.
	EventExit SessionEventType = "exit"
)

// SessionState enumerates high-level lifecycle phases.
type SessionState string

const (
	StateStarting SessionState = "starting"
	StateRunning  SessionState = "running"
	StateStopping SessionState = "stopping"
	StateExited   SessionState = "exited"
)

// SessionEvent is emitted for PTY output, lifecycle changes, and exit.
type SessionEvent struct {
	Time     time.Time
	Type     SessionEventType
	State    SessionState
	Chunk    string
	ExitCode int
	Error    error
}

// Start launches Codex inside a PTY and returns a SessionHandle that exposes
// lifecycle controls plus structured events.
func (r *SessionRunner) Start(ctx context.Context, opts StartOptions) (*SessionHandle, error) {
	if opts.Invocation.Binary == "" {
		return nil, errors.New("invocation binary is required")
	}
	if strings.TrimSpace(opts.SessionID) == "" {
		return nil, errors.New("session id is required")
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}

	runner := r
	if runner == nil {
		runner = NewSessionRunner()
	}
	if runner.launcher == nil {
		runner.launcher = realLauncher{}
	}
	if runner.preflight == nil {
		runner.preflight = defaultPreflight
	}
	if runner.now == nil {
		runner.now = time.Now
	}

	if err := runner.preflight(); err != nil {
		return nil, err
	}

	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	redactor := opts.Redactor
	if redactor == nil {
		redactor = newSecretRedactor(opts.Secrets)
	}

	events := make(chan SessionEvent, eventBufferSize)
	emitter := eventEmitter{sink: events, now: runner.now}
	emitter.state(StateStarting)

	startedAt := runner.now()
	handle, err := runner.launcher.Launch(ctx, opts.Invocation, opts.Dir, opts.Env)
	if err != nil {
		close(events)
		return nil, err
	}

	emitter.state(StateRunning)

	live := &eventLogWriter{
		target: stdout,
		emit:   emitter,
	}

	stream := newStreamWriter(live, opts.Tee, redactor)
	streamDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(stream, handle.tty)
		streamDone <- copyErr
	}()

	exec := &sessionExecution{
		runner:     runner,
		sessionID:  opts.SessionID,
		prompt:     opts.Prompt,
		invocation: opts.Invocation,
		handle:     handle,
		stream:     stream,
		streamDone: streamDone,
		events:     events,
		emitter:    emitter,
		startedAt:  startedAt,
	}
	exec.startWait()
	return &SessionHandle{exec: exec}, nil
}

type sessionExecution struct {
	runner     *SessionRunner
	sessionID  string
	prompt     string
	invocation codexexec.Invocation
	handle     *processHandle
	stream     *streamWriter
	streamDone <-chan error
	events     chan SessionEvent
	emitter    eventEmitter
	startedAt  time.Time

	waitOnce   sync.Once
	resultOnce sync.Once
	result     Result
	err        error
	outcome    chan struct{}

	softStopMu     sync.Mutex
	softStopIssued bool
	abortOnce      sync.Once
	inputMu        sync.Mutex
}

// startWait begins monitoring the Codex process and PTY stream.
func (s *sessionExecution) startWait() {
	s.outcome = make(chan struct{})
	go func() {
		defer close(s.outcome)

		waitErr := s.handle.wait()
		streamErr := <-s.streamDone
		_ = s.handle.tty.Close()

		output := s.stream.Redacted()
		completed := s.runner.now()

		res := Result{
			SessionID:   s.sessionID,
			Prompt:      s.prompt,
			Invocation:  s.invocation,
			Output:      output,
			StartedAt:   s.startedAt,
			CompletedAt: completed,
		}

		if streamErr != nil && !errors.Is(streamErr, io.EOF) && !errors.Is(streamErr, os.ErrClosed) {
			s.finish(res, fmt.Errorf("stream codex output: %w", streamErr))
			return
		}

		if waitErr != nil {
			if code, ok := exitCodeFrom(waitErr); ok {
				res.ExitCode = code
			} else {
				s.finish(res, fmt.Errorf("codex run failed: %w", waitErr))
				return
			}
		}

		s.finish(res, nil)
	}()
}

func (s *sessionExecution) finish(res Result, runErr error) {
	s.resultOnce.Do(func() {
		s.result = res
		s.err = runErr
		if runErr != nil && res.ExitCode == 0 {
			s.result.ExitCode = 1
		}
	})
	s.handle = nil
	evtErr := s.err
	if evtErr == nil {
		s.emitter.state(StateExited)
	} else {
		s.emitter.state(StateStopping)
		s.emitter.state(StateExited)
	}
	s.emitter.exit(s.result.ExitCode, evtErr)
	close(s.events)
}

func (s *sessionExecution) wait() (Result, error) {
	s.waitOnce.Do(func() {
		<-s.outcome
	})
	return s.result, s.err
}

func (s *sessionExecution) softStop(reason string) error {
	s.softStopMu.Lock()
	defer s.softStopMu.Unlock()
	if s.softStopIssued {
		return nil
	}
	if s.handle == nil || s.handle.tty == nil {
		return errors.New("tty closed")
	}
	message := formatSoftStopMessage(s.sessionID, reason)
	if _, err := io.WriteString(s.handle.tty, message); err != nil {
		return fmt.Errorf("write soft stop: %w", err)
	}
	s.softStopIssued = true
	s.emitter.state(StateStopping)
	return nil
}

func (s *sessionExecution) abort() error {
	var abortErr error
	s.abortOnce.Do(func() {
		switch {
		case s.handle == nil:
			abortErr = errors.New("session not running")
		case s.handle.signal != nil:
			abortErr = s.handle.signal(os.Interrupt)
		case s.handle.kill != nil:
			abortErr = s.handle.kill()
		default:
			abortErr = errors.New("no signal handler available")
		}
		if abortErr == nil {
			s.emitter.state(StateStopping)
		}
	})
	return abortErr
}

func (s *sessionExecution) writeInput(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if s.handle == nil || s.handle.tty == nil {
		return 0, errors.New("tty closed")
	}
	return s.handle.tty.Write(data)
}

func (s *sessionExecution) submitHint(text string) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if s.handle == nil || s.handle.tty == nil {
		return errors.New("tty closed")
	}
	message := formatHintMessage(s.sessionID, trimmed)
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if _, err := io.WriteString(s.handle.tty, message); err != nil {
		return fmt.Errorf("write hint: %w", err)
	}
	return nil
}

type eventEmitter struct {
	sink chan<- SessionEvent
	now  func() time.Time
}

func (e eventEmitter) send(evt SessionEvent) {
	if e.sink == nil {
		return
	}
	select {
	case e.sink <- evt:
	default:
	}
}

func (e eventEmitter) state(state SessionState) {
	e.send(SessionEvent{
		Time:  e.now(),
		Type:  EventStateChange,
		State: state,
	})
}

func (e eventEmitter) log(chunk string) {
	e.send(SessionEvent{
		Time:  e.now(),
		Type:  EventLogChunk,
		Chunk: chunk,
	})
}

func (e eventEmitter) exit(code int, err error) {
	e.send(SessionEvent{
		Time:     e.now(),
		Type:     EventExit,
		ExitCode: code,
		Error:    err,
	})
}

type eventLogWriter struct {
	target io.Writer
	emit   eventEmitter
}

func (w *eventLogWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.target != nil {
		if _, err := w.target.Write(p); err != nil {
			return 0, err
		}
	}
	w.emit.log(string(p))
	return len(p), nil
}

type streamWriter struct {
	live     io.Writer
	tee      io.Writer
	redactor Redactor
	builder  strings.Builder
}

func newStreamWriter(live io.Writer, tee io.Writer, redactor Redactor) *streamWriter {
	if redactor == nil {
		redactor = RedactorFunc(func(s string) string { return s })
	}
	return &streamWriter{
		live:     live,
		tee:      tee,
		redactor: redactor,
	}
}

func (w *streamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.live != nil {
		if _, err := w.live.Write(p); err != nil {
			return 0, err
		}
	}
	chunk := string(p)
	redacted := w.redactor.Redact(chunk)
	w.builder.WriteString(redacted)
	if w.tee != nil {
		if _, err := io.WriteString(w.tee, redacted); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *streamWriter) Redacted() string {
	return w.builder.String()
}

func formatSoftStopMessage(sessionID string, reason string) string {
	var sb strings.Builder
	sb.WriteString("\n\n")
	sb.WriteString(SoftStopMarker)
	sb.WriteString(" ")
	sb.WriteString(sessionID)
	sb.WriteString("\n")
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		sb.WriteString("Reason: ")
		sb.WriteString(trimmed)
		sb.WriteString("\n")
	}
	sb.WriteString("Please wrap up immediately and emit your fenced report.\n\n")
	return sb.String()
}

func formatHintMessage(sessionID string, hint string) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(hint), "\r\n", "\n"), "\n")
	var sb strings.Builder
	sb.WriteString("\n\n")
	sb.WriteString(HumanHintMarker)
	sb.WriteString(" ")
	sb.WriteString(sessionID)
	sb.WriteString("\nHint: |\n")
	for _, line := range lines {
		sb.WriteString("  ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

func randomSessionUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Set version (4) and variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3]),
		uint16(b[4])<<8|uint16(b[5]),
		uint16(b[6])<<8|uint16(b[7]),
		uint16(b[8])<<8|uint16(b[9]),
		uint64(b[10])<<40|uint64(b[11])<<32|uint64(b[12])<<24|uint64(b[13])<<16|uint64(b[14])<<8|uint64(b[15]),
	), nil
}

func fencedReportInstructions(sessionID string) string {
	return fmt.Sprintf(
		"When you finish the bead, emit a fenced report Obi can parse:\n\n```obi:%s\nstatus: success|needs_help\ncommit_msg: <single-line imperative summary>\ndetails: |\n  <multi-line explanation of everything you changed>\nescalation: <reason>  # required when status=needs_help\n```\n\nIf you receive a line containing %s, finish your current action and emit the fenced report immediately.\n\nAfter the fenced report, also output the legacy footer so older tooling continues to work:\nSTATUS: success|needs_help\nCOMMIT_MSG:\n<same multi-line summary as above>\nESCALATION: <reason>  # only if status=needs_help",
		sessionID,
		SoftStopMarker,
	)
}

type launcher interface {
	Launch(ctx context.Context, inv codexexec.Invocation, dir string, env []string) (*processHandle, error)
}

type processHandle struct {
	tty    io.ReadWriteCloser
	wait   func() error
	kill   func() error
	signal func(os.Signal) error
}

type realLauncher struct{}

func (realLauncher) Launch(ctx context.Context, inv codexexec.Invocation, dir string, env []string) (*processHandle, error) {
	cmd := exec.CommandContext(ctx, inv.Binary, inv.Args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	tty, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("start codex PTY: %w", err)
	}

	return &processHandle{
		tty: tty,
		wait: func() error {
			return cmd.Wait()
		},
		kill: func() error {
			if cmd.Process == nil {
				return nil
			}
			return cmd.Process.Kill()
		},
		signal: func(sig os.Signal) error {
			if cmd.Process == nil {
				return nil
			}
			return cmd.Process.Signal(sig)
		},
	}, nil
}

func defaultPreflight() error {
	if usePipeLauncher() {
		return nil
	}
	if runtime.GOOS == "windows" {
		return errors.New("interactive mode requires a Unix-style PTY; Windows is not supported")
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "android" {
		if err := requireDevice("/dev/ptmx"); err != nil {
			return err
		}
	}
	return nil
}

func requireDevice(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("interactive mode requires %s (PTY unavailable)", path)
		}
		return fmt.Errorf("check %s: %w", path, err)
	}
	return nil
}

type exitCoder interface {
	ExitCode() int
}

type pipeLauncher struct{}

func (pipeLauncher) Launch(ctx context.Context, inv codexexec.Invocation, dir string, env []string) (*processHandle, error) {
	cmd := exec.CommandContext(ctx, inv.Binary, inv.Args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	reader, writer := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(writer, stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(writer, stderr)
	}()
	go func() {
		wg.Wait()
		writer.Close()
	}()

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	tty := &pipeTTY{r: reader, w: stdin}
	return &processHandle{
		tty: tty,
		wait: func() error {
			return cmd.Wait()
		},
		kill: func() error {
			if cmd.Process == nil {
				return nil
			}
			return cmd.Process.Kill()
		},
		signal: func(sig os.Signal) error {
			if cmd.Process == nil {
				return nil
			}
			return cmd.Process.Signal(sig)
		},
	}, nil
}

type pipeTTY struct {
	r io.ReadCloser
	w io.WriteCloser
}

func (p *pipeTTY) Read(b []byte) (int, error) {
	return p.r.Read(b)
}

func (p *pipeTTY) Write(b []byte) (int, error) {
	return p.w.Write(b)
}

func (p *pipeTTY) Close() error {
	var errR, errW error
	if p.r != nil {
		errR = p.r.Close()
	}
	if p.w != nil {
		errW = p.w.Close()
	}
	if errR != nil {
		return errR
	}
	return errW
}

func exitCodeFrom(err error) (int, bool) {
	var coder exitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode(), true
	}
	return 0, false
}
