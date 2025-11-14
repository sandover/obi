package app

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type operatorEventKind string

const (
	operatorEventHint     operatorEventKind = "hint"
	operatorEventSoftStop operatorEventKind = "soft_stop"
)

type operatorEvent struct {
	Kind    operatorEventKind
	Message string
	Time    time.Time
}

type operatorLog struct {
	mu       sync.Mutex
	entries  []operatorEvent
	now      func() time.Time
	writer   io.Writer
	writerMu sync.Mutex
}

func newOperatorLog(writer io.Writer) *operatorLog {
	return &operatorLog{
		now:    time.Now,
		writer: writer,
	}
}

func (l *operatorLog) record(kind operatorEventKind, message string) {
	if l == nil {
		return
	}
	if strings.TrimSpace(message) == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, operatorEvent{
		Kind:    kind,
		Message: message,
		Time:    l.now(),
	})
	l.writeMirror(kind, message)
}

func (l *operatorLog) events() []operatorEvent {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]operatorEvent, len(l.entries))
	copy(out, l.entries)
	return out
}

func (l *operatorLog) ledgerEvents(secrets []string) []operatorLedgerEvent {
	events := l.events()
	if len(events) == 0 {
		return nil
	}
	var out []operatorLedgerEvent
	for _, evt := range events {
		message := strings.TrimSpace(evt.Message)
		if message == "" {
			continue
		}
		redacted, _ := redactText(message, secrets)
		out = append(out, operatorLedgerEvent{
			Kind:    string(evt.Kind),
			Message: redacted,
			Time:    evt.Time,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (l *operatorLog) writeMirror(kind operatorEventKind, message string) {
	if l == nil || l.writer == nil {
		return
	}
	label := "operator"
	switch kind {
	case operatorEventHint:
		label = "operator hint"
	case operatorEventSoftStop:
		label = "operator soft-stop"
	}
	line := fmt.Sprintf("\n[obi %s] %s\n", label, message)
	l.writerMu.Lock()
	defer l.writerMu.Unlock()
	_, _ = io.WriteString(l.writer, line)
}
