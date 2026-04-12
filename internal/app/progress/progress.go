package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type Mode string

const (
	ModeAuto  Mode = "auto"
	ModeTTY   Mode = "tty"
	ModePlain Mode = "plain"
	ModeQuiet Mode = "quiet"
)

type Reporter interface {
	StartStage(name string, total int)
	Advance(delta int)
	Status(message string)
	FinishStage(message string)
}

type NoopReporter struct{}

func (NoopReporter) StartStage(string, int) {}
func (NoopReporter) Advance(int)            {}
func (NoopReporter) Status(string)          {}
func (NoopReporter) FinishStage(string)     {}

type StderrReporter struct {
	mu               sync.Mutex
	writer           io.Writer
	mode             Mode
	stage            string
	total            int
	current          int
	status           string
	lastWidth        int
	lastEmit         time.Time
	lastEmittedCount int
}

func NewReporter(mode string) Reporter {
	resolved := normalizeMode(mode, isTerminal(os.Stderr))
	if resolved == ModeQuiet {
		return NoopReporter{}
	}
	return &StderrReporter{
		writer: os.Stderr,
		mode:   resolved,
	}
}

func NewStderrReporter(mode ...string) Reporter {
	if len(mode) > 0 {
		return NewReporter(mode[0])
	}
	return NewReporter(string(ModeAuto))
}

func (r *StderrReporter) StartStage(name string, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLineLocked()
	r.stage = name
	r.total = total
	r.current = 0
	r.status = ""
	r.lastEmit = time.Time{}
	r.lastEmittedCount = 0
	r.renderLocked(true)
}

func (r *StderrReporter) Advance(delta int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if delta > 0 {
		r.current += delta
	}
	r.renderLocked(false)
}

func (r *StderrReporter) Status(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = strings.TrimSpace(message)
	r.renderLocked(false)
}

func (r *StderrReporter) FinishStage(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(message) != "" {
		r.status = strings.TrimSpace(message)
	}
	line := r.lineLocked()
	switch r.mode {
	case ModeTTY:
		fmt.Fprintf(r.writer, "\r%s\n", padRight(line, r.lastWidth))
		r.lastWidth = 0
	default:
		fmt.Fprintln(r.writer, line)
	}
	r.stage = ""
	r.total = 0
	r.current = 0
	r.status = ""
	r.lastEmit = time.Now()
	r.lastEmittedCount = 0
}

func (r *StderrReporter) renderLocked(force bool) {
	if r.stage == "" {
		return
	}
	line := r.lineLocked()
	switch r.mode {
	case ModeTTY:
		fmt.Fprintf(r.writer, "\r%s", padRight(line, r.lastWidth))
		if len(line) > r.lastWidth {
			r.lastWidth = len(line)
		}
	default:
		if !force && !r.shouldEmitPlainLocked() {
			return
		}
		fmt.Fprintln(r.writer, line)
		r.lastEmit = time.Now()
		r.lastEmittedCount = r.current
	}
}

func (r *StderrReporter) shouldEmitPlainLocked() bool {
	if r.lastEmit.IsZero() {
		return true
	}
	if r.current-r.lastEmittedCount >= 100 {
		return true
	}
	return time.Since(r.lastEmit) >= 250*time.Millisecond
}

func (r *StderrReporter) lineLocked() string {
	switch r.mode {
	case ModeTTY:
		if r.total > 0 {
			return fmt.Sprintf("%s %s %d/%d %s", progressBar(r.current, r.total), r.stage, r.current, r.total, r.status)
		}
		return fmt.Sprintf("[....] %s %d %s", r.stage, r.current, r.status)
	default:
		if r.total > 0 {
			return fmt.Sprintf("stage=%s current=%d total=%d status=%s", r.stage, r.current, r.total, strings.TrimSpace(r.status))
		}
		return fmt.Sprintf("stage=%s current=%d status=%s", r.stage, r.current, strings.TrimSpace(r.status))
	}
}

func (r *StderrReporter) finishLineLocked() {
	if r.mode == ModeTTY && r.lastWidth > 0 {
		fmt.Fprintln(r.writer)
		r.lastWidth = 0
	}
}

func progressBar(current, total int) string {
	if total <= 0 {
		return "[....]"
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	width := 20
	filled := current * width / total
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat(".", width-filled) + "]"
}

func padRight(line string, width int) string {
	if len(line) >= width {
		return line
	}
	return line + strings.Repeat(" ", width-len(line))
}

func normalizeMode(raw string, terminal bool) Mode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(ModeAuto):
		if terminal {
			return ModeTTY
		}
		return ModePlain
	case string(ModeTTY):
		return ModeTTY
	case string(ModePlain):
		return ModePlain
	case string(ModeQuiet):
		return ModeQuiet
	default:
		if terminal {
			return ModeTTY
		}
		return ModePlain
	}
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
