package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/interactive"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/tui"
)

type sessionDisplay struct {
	shell       *tui.Shell
	cancel      context.CancelFunc
	done        chan error
	release     chan struct{}
	inputCancel context.CancelFunc
	inputDone   chan error
	stopOnce    sync.Once
}

func (d *sessionDisplay) Stop() {
	if d == nil {
		return
	}
	d.stopOnce.Do(func() {
		close(d.release)
		if d.inputCancel != nil {
			d.inputCancel()
		}
		if d.inputDone != nil {
			if err := <-d.inputDone; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				fmt.Fprintf(os.Stderr, "tui input: %v\n", err)
			}
		}
		d.cancel()
		if d.done != nil {
			if err := <-d.done; err != nil && err != context.Canceled {
				fmt.Fprintf(os.Stderr, "tui shell: %v\n", err)
			}
		}
	})
}

func (d *sessionDisplay) UpdateStatus(update func(*tui.StatusLine)) {
	if d == nil || d.shell == nil || update == nil {
		return
	}
	d.shell.UpdateStatus(update)
}

func (d *sessionDisplay) notifyEvent(kind operatorEventKind, message string) {
	if d == nil {
		return
	}
	text := strings.TrimSpace(message)
	if text == "" {
		return
	}
	chunk := fmt.Sprintf("\n[obi %s] %s\n", kind, text)
	if d.shell != nil {
		d.shell.HandleEvent(interactive.SessionEvent{Type: interactive.EventLogChunk, Chunk: chunk})
	} else {
		fmt.Fprintf(os.Stderr, "%s", chunk)
	}
}

func startSessionTUI(handle *interactive.SessionHandle, plan sessionPlan, log *operatorLog) (*sessionDisplay, error) {
	if handle == nil {
		return nil, nil
	}
	src := handle.Events()
	if src == nil {
		return nil, nil
	}

	header := fmt.Sprintf("Obi session · %s (%s)", plan.EpicName, plan.EpicID)
	shell := tui.NewShell(
		tui.WithHeader(header),
		tui.WithFooterHints([]string{"p: pause", "h: hint", "s: soft stop", "q: abort"}),
	)
	shell.UpdateStatus(func(line *tui.StatusLine) {
		line.EpicAlias = plan.Alias
		if strings.TrimSpace(line.EpicAlias) == "" {
			line.EpicAlias = plan.EpicName
		}
		line.EpicID = plan.EpicID
		line.RunStatus = string(interactive.StateStarting)
		line.StartedAt = time.Now()
	})

	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	events := make(chan interactive.SessionEvent, 64)

	go func() {
		defer close(events)
		for {
			select {
			case evt, ok := <-src:
				if !ok {
					<-release
					return
				}
				events <- evt
			case <-release:
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- shell.Run(ctx, events)
	}()

	// Detect immediate failure (e.g., raw-mode errors) before continuing.
	select {
	case err := <-done:
		close(release)
		cancel()
		if err != nil {
			return nil, err
		}
		return nil, nil
	default:
	}

	shell.UpdateStatus(func(line *tui.StatusLine) {
		line.RunStatus = string(interactive.StateRunning)
	})

	display := &sessionDisplay{
		shell:   shell,
		cancel:  cancel,
		done:    done,
		release: release,
	}

	controls := &sessionControlsAdapter{
		session: handle,
		log:     log,
		notify:  display.notifyEvent,
	}
	hintSubmitter := &hintSubmitterAdapter{
		session: handle,
		log:     log,
		notify:  display.notifyEvent,
	}
	router := tui.NewInputRouter(controls, shell, tui.WithHintSubmitter(hintSubmitter))

	inputCtx, inputCancel := context.WithCancel(context.Background())
	display.inputCancel = inputCancel
	display.inputDone = make(chan error, 1)
	go func() {
		reader := shell.InputReader()
		if reader == nil {
			display.inputDone <- io.EOF
			return
		}
		display.inputDone <- router.Run(inputCtx, reader)
	}()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				shell.RequestRender()
			}
		}
	}()

	return display, nil
}

type eventNotifier func(operatorEventKind, string)

type sessionControlsAdapter struct {
	session *interactive.SessionHandle
	log     *operatorLog
	notify  eventNotifier
}

func (s *sessionControlsAdapter) WriteInput(data []byte) (int, error) {
	if s.session == nil {
		return 0, errors.New("session controls unavailable")
	}
	return s.session.WriteInput(data)
}

func (s *sessionControlsAdapter) SoftStop(reason string) error {
	if s.session == nil {
		return errors.New("session controls unavailable")
	}
	if err := s.session.SoftStop(reason); err != nil {
		return err
	}
	s.log.record(operatorEventSoftStop, reason)
	if s.notify != nil {
		s.notify(operatorEventSoftStop, fmt.Sprintf("Soft stop requested: %s", reason))
	}
	return nil
}

func (s *sessionControlsAdapter) Abort() error {
	if s.session == nil {
		return errors.New("session controls unavailable")
	}
	return s.session.Abort()
}

type hintSubmitterAdapter struct {
	session *interactive.SessionHandle
	log     *operatorLog
	notify  eventNotifier
}

func (h *hintSubmitterAdapter) SubmitHint(text string) error {
	if h.session == nil {
		return errors.New("session controls unavailable")
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if err := h.session.SubmitHint(trimmed); err != nil {
		return err
	}
	h.log.record(operatorEventHint, trimmed)
	if h.notify != nil {
		h.notify(operatorEventHint, fmt.Sprintf("Hint sent: %s", trimmed))
	}
	return nil
}
