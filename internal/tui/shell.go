package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/interactive"
)

const (
	headerLines = 4
)

var helpOverlayLines = []string{
	"Help:",
	"p - Pause/resume log output",
	"h - Enter hint mode",
	"s - Request soft stop",
	"q - Abort Codex session",
	"? - Toggle this overlay",
}

// TokenUsage captures Codex token metrics shown in the header.
type TokenUsage struct {
	Used     int
	Limit    int
	HasUsed  bool
	HasLimit bool
}

// StatusLine holds the metadata rendered in the shell header.
type StatusLine struct {
	EpicAlias string
	EpicID    string
	BeadID    string
	BeadTitle string
	RunStatus string
	StartedAt time.Time
	Tokens    TokenUsage
}

func (s StatusLine) beadSummary() string {
	id := strings.TrimSpace(s.BeadID)
	title := strings.TrimSpace(s.BeadTitle)
	switch {
	case id == "" && title == "":
		return "pending selection"
	case id == "":
		return title
	case title == "":
		return id
	default:
		return fmt.Sprintf("%s - %s", id, title)
	}
}

func (s StatusLine) tokensSummary() string {
	used := "--"
	if s.Tokens.HasUsed {
		used = strconv.Itoa(s.Tokens.Used)
	}
	limit := "--"
	if s.Tokens.HasLimit {
		limit = strconv.Itoa(s.Tokens.Limit)
	}
	return fmt.Sprintf("%s/%s", used, limit)
}

func (s StatusLine) elapsed(now time.Time) string {
	if s.StartedAt.IsZero() || now.Before(s.StartedAt) {
		return "00:00"
	}
	dur := now.Sub(s.StartedAt)
	return formatElapsed(dur)
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d.Round(time.Second) / time.Second)
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

// Shell controls the raw-mode terminal UI for interactive Codex sessions.
type Shell struct {
	in    *os.File
	out   io.Writer
	term  termAdapter
	fd    int
	state *termState

	height int
	width  int

	header string
	footer []string

	pane *logPane

	renderCh chan struct{}

	mu         sync.Mutex
	session    interactive.SessionState
	exitLabel  string
	termErr    error
	paused     bool
	help       bool
	hintActive bool
	hintText   string
	status     StatusLine
}

// Option configures a Shell.
type Option func(*Shell)

// WithHeader overrides the default header text.
func WithHeader(text string) Option {
	return func(s *Shell) {
		s.header = text
	}
}

// WithFooterHints sets the footer hotkey legend entries.
func WithFooterHints(hints []string) Option {
	return func(s *Shell) {
		s.footer = append([]string{}, hints...)
	}
}

// WithMaxLogs caps the number of log lines buffered in the pane.
func WithMaxLogs(max int) Option {
	return func(s *Shell) {
		s.ensurePane()
		s.pane.setMax(max)
	}
}

// WithIO overrides the default stdin/stdout handles.
func WithIO(in *os.File, out io.Writer) Option {
	return func(s *Shell) {
		if in != nil {
			s.in = in
		}
		if out != nil {
			s.out = out
		}
	}
}

func withTerminal(term termAdapter) Option {
	return func(s *Shell) {
		s.term = term
	}
}

// InputReader exposes the shell's input handle for external routers.
func (s *Shell) InputReader() io.Reader {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.in == nil {
		s.in = os.Stdin
	}
	return s.in
}

// UpdateStatus applies a mutation to the header metadata and triggers a refresh.
func (s *Shell) UpdateStatus(update func(*StatusLine)) {
	if s == nil || update == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.status)
	s.requestRenderLocked()
}

// NewShell constructs a Shell ready to render interactive session output.
func NewShell(opts ...Option) *Shell {
	sh := &Shell{
		header: "Obi Interactive Session",
		footer: []string{
			"Ctrl+C: soft stop",
			"Ctrl+C again: abort session",
		},
		session:  interactive.StateStarting,
		fd:       -1,
		renderCh: make(chan struct{}, 1),
		status: StatusLine{
			RunStatus: string(interactive.StateStarting),
		},
	}
	for _, opt := range opts {
		opt(sh)
	}
	sh.ensurePane()
	if sh.in == nil {
		sh.in = os.Stdin
	}
	if sh.out == nil {
		sh.out = os.Stdout
	}
	if sh.term == nil {
		sh.term = systemTerminal{}
	}
	return sh
}

func (s *Shell) ensurePane() {
	if s.pane == nil {
		s.pane = newLogPane(defaultMaxLogs)
	}
}

func (s *Shell) requestRenderLocked() {
	if s.renderCh == nil {
		return
	}
	select {
	case s.renderCh <- struct{}{}:
	default:
	}
}

// RequestRender schedules a refresh even when no Codex events are flowing.
func (s *Shell) RequestRender() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestRenderLocked()
}

// Run consumes session events, renders the UI, and manages terminal state.
func (s *Shell) Run(ctx context.Context, events <-chan interactive.SessionEvent) error {
	if events == nil {
		return errors.New("events channel is required")
	}
	if err := s.enterRawMode(); err != nil {
		return err
	}
	defer s.restoreTerminal()

	if err := s.render(); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.renderCh:
			if err := s.render(); err != nil {
				return err
			}
		case evt, ok := <-events:
			if !ok {
				s.flushPending()
				return s.render()
			}
			s.HandleEvent(evt)
			if err := s.render(); err != nil {
				return err
			}
		}
	}
}

func (s *Shell) flushPending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pane.flushPartial()
}

// HandleEvent applies an event to the shell state. Safe for concurrent use.
func (s *Shell) HandleEvent(evt interactive.SessionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch evt.Type {
	case interactive.EventLogChunk:
		s.pane.append(evt.Chunk)
	case interactive.EventStateChange:
		if evt.State != "" {
			s.session = evt.State
		}
	case interactive.EventExit:
		s.pane.flushPartial()
		s.exitLabel = formatExit(evt)
		s.session = interactive.StateExited
	default:
		// ignore
	}
}

// Scroll adjusts the log pane offset (positive scrolls upward).
func (s *Shell) Scroll(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pane.scrollBy(delta)
}

// TogglePause flips the paused state and returns the updated value.
func (s *Shell) TogglePause() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setPausedLocked(!s.paused)
}

// SetPaused updates the paused state explicitly.
func (s *Shell) SetPaused(paused bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setPausedLocked(paused)
}

// Paused reports whether the log pane is currently paused.
func (s *Shell) Paused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// SetHintInput toggles hint-entry mode and updates the visible text.
func (s *Shell) SetHintInput(active bool, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hintActive = active
	if active {
		s.hintText = text
	} else {
		s.hintText = ""
	}
	s.requestRenderLocked()
}

// HintInput reports the currently visible hint text.
func (s *Shell) HintInput() (text string, active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hintText, s.hintActive
}

// ToggleHelp flips the help overlay visibility, returning the new state.
func (s *Shell) ToggleHelp() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.help = !s.help
	s.requestRenderLocked()
	return s.help
}

// SetHelpVisible forces the help overlay to a specific state.
func (s *Shell) SetHelpVisible(visible bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.help = visible
	s.requestRenderLocked()
	return s.help
}

// HelpVisible reports whether the overlay is currently on screen.
func (s *Shell) HelpVisible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.help
}

func (s *Shell) setPausedLocked(target bool) bool {
	s.ensurePane()
	if s.pane != nil {
		target = s.pane.setPaused(target)
	}
	if !target && s.pane != nil {
		s.pane.resetScroll()
	}
	s.paused = target
	s.requestRenderLocked()
	return s.paused
}

func (s *Shell) enterRawMode() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.term == nil {
		s.term = systemTerminal{}
	}
	if s.in == nil {
		s.in = os.Stdin
	}
	if s.out == nil {
		s.out = os.Stdout
	}

	fd := int(s.in.Fd())
	st, err := s.term.makeRaw(fd)
	if err != nil {
		return fmt.Errorf("enable raw mode: %w", err)
	}
	s.fd = fd
	s.state = st
	s.writeAnsi("\x1b[?25l") // hide cursor
	s.measureSizeLocked()
	return nil
}

func (s *Shell) restoreTerminal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.term != nil && s.state != nil && s.fd >= 0 {
		_ = s.term.restore(s.fd, s.state)
	}
	s.writeAnsi("\x1b[?25h\x1b[0m")
}

func (s *Shell) writeAnsi(seq string) {
	if s.out == nil || seq == "" {
		return
	}
	_, _ = io.WriteString(s.out, seq)
}

func (s *Shell) measureSizeLocked() {
	if s.term == nil || s.fd < 0 {
		return
	}
	if w, h, err := s.term.getSize(s.fd); err == nil && w > 0 && h > 0 {
		s.width = w
		s.height = h
		return
	}
	if s.width == 0 {
		s.width = 80
	}
	if s.height == 0 {
		s.height = 24
	}
}

func (s *Shell) render() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.measureSizeLocked()

	hintLines := s.hintLineCountLocked()
	footerHeight := s.footerHeightLocked()
	viewHeight := s.height - headerLines - footerHeight - hintLines
	if viewHeight < 1 {
		viewHeight = 1
	}
	logs := s.pane.visible(viewHeight)

	var buf bytes.Buffer
	buf.WriteString("\x1b[2J\x1b[H")
	buf.WriteString(s.renderHeaderLocked())
	for _, line := range logs {
		buf.WriteString(truncateToWidth(line, s.width))
		buf.WriteByte('\n')
	}
	padLines := viewHeight - len(logs)
	for i := 0; i < padLines; i++ {
		buf.WriteByte('\n')
	}
	if hintLines > 0 {
		buf.WriteString(s.renderHintLocked())
	}
	buf.WriteByte('\n')
	buf.WriteString(s.renderFooterLocked())

	if _, err := buf.WriteTo(s.out); err != nil {
		return fmt.Errorf("render tui: %w", err)
	}
	return nil
}

func (s *Shell) renderHeaderLocked() string {
	title := s.header
	if title == "" {
		title = "Obi Interactive Session"
	}
	alias := strings.TrimSpace(s.status.EpicAlias)
	if alias == "" {
		alias = "n/a"
	}
	epicID := strings.TrimSpace(s.status.EpicID)
	if epicID == "" {
		epicID = "-"
	}
	bead := s.status.beadSummary()
	line2 := fmt.Sprintf("Epic: %s (%s) | Bead: %s", alias, epicID, bead)

	statusText := strings.TrimSpace(s.status.RunStatus)
	if statusText == "" {
		statusText = string(s.session)
		if statusText == "" {
			statusText = "unknown"
		}
	}
	segments := []string{statusText}
	if s.exitLabel != "" {
		segments = append(segments, s.exitLabel)
	}
	if s.paused {
		segments = append(segments, "PAUSED")
	}
	status := strings.Join(segments, "  *  ")
	elapsed := s.status.elapsed(time.Now())
	tokens := s.status.tokensSummary()
	line3 := fmt.Sprintf("Status: %s | Elapsed: %s | Tokens: %s", status, elapsed, tokens)
	return fmt.Sprintf("%s\n%s\n%s\n\n",
		truncateToWidth(title, s.width),
		truncateToWidth(line2, s.width),
		truncateToWidth(line3, s.width),
	)
}

func (s *Shell) renderFooterLocked() string {
	var lines []string
	if len(s.footer) > 0 {
		lines = append(lines, fmt.Sprintf("Hotkeys: %s", strings.Join(s.footer, "  *  ")))
	}
	if s.help {
		lines = append(lines, helpOverlayLines...)
	}
	if len(lines) == 0 {
		return "\n"
	}
	return strings.Join(lines, "\n") + "\n"
}

func (s *Shell) footerHeightLocked() int {
	lines := s.footerLineCountLocked()
	if lines == 0 {
		lines = 1
	}
	// Account for the blank spacer preceding the footer block.
	return 1 + lines
}

func (s *Shell) footerLineCountLocked() int {
	lines := 0
	if len(s.footer) > 0 {
		lines++
	}
	if s.help {
		lines += len(helpOverlayLines)
	}
	return lines
}

func (s *Shell) hintLineCountLocked() int {
	if s.hintActive {
		return 1
	}
	return 0
}

func (s *Shell) renderHintLocked() string {
	if !s.hintActive {
		return ""
	}
	line := fmt.Sprintf("Hint (Enter=send, Esc=cancel): %s", s.hintText)
	return truncateToWidth(line, s.width) + "\n"
}

func formatExit(evt interactive.SessionEvent) string {
	if evt.Error != nil {
		return fmt.Sprintf("exit %d (%v)", evt.ExitCode, evt.Error)
	}
	return fmt.Sprintf("exit %d", evt.ExitCode)
}

func truncateToWidth(line string, width int) string {
	if width <= 0 {
		return line
	}
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	return string(runes[:width])
}

type termAdapter interface {
	makeRaw(fd int) (*termState, error)
	restore(fd int, state *termState) error
	getSize(fd int) (int, int, error)
}
