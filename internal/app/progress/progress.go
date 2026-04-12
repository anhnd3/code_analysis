package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
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
	mu         sync.Mutex
	writer     io.Writer
	isTerminal bool
	stage      string
	total      int
	current    int
	status     string
	lastWidth  int
}

func NewStderrReporter() Reporter {
	return &StderrReporter{
		writer:     os.Stderr,
		isTerminal: isTerminal(os.Stderr),
	}
}

func (r *StderrReporter) StartStage(name string, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLineLocked()
	r.stage = name
	r.total = total
	r.current = 0
	r.status = ""
	r.renderLocked()
}

func (r *StderrReporter) Advance(delta int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if delta > 0 {
		r.current += delta
	}
	r.renderLocked()
}

func (r *StderrReporter) Status(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = strings.TrimSpace(message)
	r.renderLocked()
}

func (r *StderrReporter) FinishStage(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(message) != "" {
		r.status = strings.TrimSpace(message)
	}
	line := r.lineLocked()
	if r.isTerminal {
		fmt.Fprintf(r.writer, "\r%s\n", padRight(line, r.lastWidth))
		r.lastWidth = 0
	} else {
		fmt.Fprintln(r.writer, line)
	}
	r.stage = ""
	r.total = 0
	r.current = 0
	r.status = ""
}

func (r *StderrReporter) renderLocked() {
	if r.stage == "" {
		return
	}
	line := r.lineLocked()
	if r.isTerminal {
		fmt.Fprintf(r.writer, "\r%s", padRight(line, r.lastWidth))
		if len(line) > r.lastWidth {
			r.lastWidth = len(line)
		}
		return
	}
	fmt.Fprintln(r.writer, line)
}

func (r *StderrReporter) lineLocked() string {
	if r.total > 0 {
		return fmt.Sprintf("%s %s %d/%d %s", progressBar(r.current, r.total), r.stage, r.current, r.total, r.status)
	}
	return fmt.Sprintf("[....] %s %d %s", r.stage, r.current, r.status)
}

func (r *StderrReporter) finishLineLocked() {
	if r.isTerminal && r.lastWidth > 0 {
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

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
