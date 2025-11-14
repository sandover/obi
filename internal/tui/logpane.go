package tui

import (
	"strings"
)

const defaultMaxLogs = 500

type logPane struct {
	maxLines      int
	lines         []string
	partial       string
	scroll        int
	paused        bool
	pausedLen     int
	pausedPartial string
}

func newLogPane(max int) *logPane {
	if max <= 0 {
		max = defaultMaxLogs
	}
	return &logPane{maxLines: max}
}

func (p *logPane) append(chunk string) {
	if chunk == "" {
		return
	}
	chunk = strings.ReplaceAll(chunk, "\r\n", "\n")
	chunk = strings.ReplaceAll(chunk, "\r", "\n")

	text := p.partial + chunk
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return
	}

	p.partial = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		p.addLine(line)
	}
}

func (p *logPane) addLine(line string) {
	p.lines = append(p.lines, line)
	if len(p.lines) > p.maxLines {
		drop := len(p.lines) - p.maxLines
		if drop > len(p.lines) {
			drop = len(p.lines)
		}
		p.lines = p.lines[drop:]
	}
	if p.paused && p.pausedLen > len(p.lines) {
		p.pausedLen = len(p.lines)
	}
	p.clampScroll()
}

func (p *logPane) clampScroll() {
	if p.scroll < 0 {
		p.scroll = 0
		return
	}
	total := p.bufferLength()
	if total == 0 {
		p.scroll = 0
		return
	}
	maxScroll := total - 1
	if p.scroll > maxScroll {
		p.scroll = maxScroll
		if p.scroll < 0 {
			p.scroll = 0
		}
	}
}

func (p *logPane) flushPartial() {
	if p.partial == "" {
		return
	}
	p.addLine(p.partial)
	p.partial = ""
}

func (p *logPane) visible(height int) []string {
	if height <= 0 {
		return nil
	}
	data := p.currentData()
	if len(data) == 0 {
		return nil
	}

	end := len(data) - p.scroll
	if end < 0 {
		end = 0
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	if start > end {
		start = end
	}
	slice := data[start:end]
	return append([]string{}, slice...)
}

func (p *logPane) scrollBy(delta int) {
	if delta == 0 {
		return
	}
	p.scroll += delta
	p.clampScroll()
}

func (p *logPane) resetScroll() {
	p.scroll = 0
}

func (p *logPane) setMax(max int) {
	if max <= 0 {
		max = defaultMaxLogs
	}
	p.maxLines = max
	if len(p.lines) > p.maxLines {
		p.lines = p.lines[len(p.lines)-p.maxLines:]
	}
	if p.paused && p.pausedLen > len(p.lines) {
		p.pausedLen = len(p.lines)
	}
	p.clampScroll()
}

func (p *logPane) setPaused(paused bool) bool {
	if p == nil {
		return false
	}
	if p.paused == paused {
		return p.paused
	}
	p.paused = paused
	if paused {
		p.pausedLen = len(p.lines)
		p.pausedPartial = p.partial
	} else {
		p.pausedLen = 0
		p.pausedPartial = ""
	}
	p.clampScroll()
	return p.paused
}

func (p *logPane) bufferLength() int {
	if p == nil {
		return 0
	}
	total := len(p.lines)
	if p.paused {
		limit := p.pausedLen
		if limit < 0 {
			limit = 0
		}
		if limit > len(p.lines) {
			limit = len(p.lines)
		}
		total = limit
		if p.pausedPartial != "" {
			total++
		}
		return total
	}
	if p.partial != "" {
		total++
	}
	return total
}

func (p *logPane) currentData() []string {
	if p == nil {
		return nil
	}
	data := append([]string{}, p.lines...)
	if p.paused {
		limit := p.pausedLen
		if limit < 0 {
			limit = 0
		}
		if limit > len(data) {
			limit = len(data)
		}
		data = data[:limit]
		if p.pausedPartial != "" {
			data = append(data, p.pausedPartial)
		}
		return data
	}
	if p.partial != "" {
		data = append(data, p.partial)
	}
	return data
}
