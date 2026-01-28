package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Step represents a single step in a multi-step process
type Step struct {
	Name   string
	Status StepStatus
}

type StepStatus int

const (
	StepPending StepStatus = iota
	StepRunning
	StepComplete
	StepFailed
)

// StepTracker displays a list of steps with their status
type StepTracker struct {
	steps   []Step
	current int
	w       io.Writer
}

// NewStepTracker creates a new step tracker
func NewStepTracker(w io.Writer, steps ...string) *StepTracker {
	s := make([]Step, len(steps))
	for i, name := range steps {
		s[i] = Step{Name: name, Status: StepPending}
	}
	return &StepTracker{steps: s, current: 0, w: w}
}

// Start marks the current step as running and prints it
func (t *StepTracker) Start(name string) {
	for i := range t.steps {
		if t.steps[i].Name == name {
			t.current = i
			t.steps[i].Status = StepRunning
			break
		}
	}
	fmt.Fprintf(t.w, "%s %s\n", Highlight.Render("→"), name)
}

// Complete marks the current step as complete
func (t *StepTracker) Complete(name string) {
	for i := range t.steps {
		if t.steps[i].Name == name {
			t.steps[i].Status = StepComplete
			break
		}
	}
}

// Fail marks a step as failed
func (t *StepTracker) Fail(name string) {
	for i := range t.steps {
		if t.steps[i].Name == name {
			t.steps[i].Status = StepFailed
			break
		}
	}
	fmt.Fprintf(t.w, "  %s\n", ErrorStyle.Render("failed"))
}

// ProgressWriter wraps an io.Writer to show download progress with a nice bar
type ProgressWriter struct {
	total   int64
	written int64
	w       io.Writer
	prog    progress.Model
	label   string
	started bool
}

// NewProgressWriter creates a progress writer for downloads
func NewProgressWriter(w io.Writer, total int64, label string) *ProgressWriter {
	prog := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)
	return &ProgressWriter{
		total: total,
		w:     w,
		prog:  prog,
		label: label,
	}
}

// Write implements io.Writer
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)

	if pw.total > 0 {
		pct := float64(pw.written) / float64(pw.total)
		bar := pw.prog.ViewAs(pct)
		stats := fmt.Sprintf("%s / %s", formatBytes(pw.written), formatBytes(pw.total))
		fmt.Fprintf(pw.w, "\r  %s %s %s", pw.label, bar, MutedStyle.Render(stats))
	} else {
		fmt.Fprintf(pw.w, "\r  %s %s", pw.label, MutedStyle.Render(formatBytes(pw.written)))
	}

	return n, nil
}

// Finish completes the progress display
func (pw *ProgressWriter) Finish() {
	fmt.Fprintln(pw.w)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// SpinnerModel is a bubbletea model for showing a spinner with a message
type SpinnerModel struct {
	spinner spinner.Model
	message string
	done    bool
	err     error
}

// NewSpinner creates a new spinner model
func NewSpinner(message string) SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(Primary)
	return SpinnerModel{spinner: s, message: message}
}

func (m SpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case doneMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	}
	return m, nil
}

func (m SpinnerModel) View() string {
	if m.done {
		if m.err != nil {
			return fmt.Sprintf("%s %s\n", SymbolError, m.message)
		}
		return fmt.Sprintf("%s %s\n", SymbolSuccess, m.message)
	}
	return fmt.Sprintf("%s %s\n", m.spinner.View(), m.message)
}

type doneMsg struct{ err error }

// RunWithSpinner runs a function with a spinner display
func RunWithSpinner(message string, fn func() error) error {
	p := tea.NewProgram(NewSpinner(message))

	go func() {
		err := fn()
		p.Send(doneMsg{err: err})
	}()

	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// PrintHeader prints a styled header
func PrintHeader(w io.Writer, title string) {
	fmt.Fprintln(w, Title.Render(title))
	fmt.Fprintln(w, strings.Repeat("─", len(title)))
}

// PrintSuccess prints a success message
func PrintSuccess(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(w, "%s %s\n", SymbolSuccess, msg)
}

// PrintError prints an error message
func PrintError(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(w, "%s %s\n", SymbolError, ErrorStyle.Render(msg))
}

// PrintInfo prints an info message
func PrintInfo(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(w, "%s %s\n", SymbolInfo, msg)
}

// PrintStep prints a step with indentation
func PrintStep(w io.Writer, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(w, "  %s %s\n", MutedStyle.Render("•"), msg)
}

// WaitingDots shows animated waiting dots
type WaitingDots struct {
	w       io.Writer
	message string
	done    chan struct{}
}

// NewWaitingDots creates a new waiting dots display
func NewWaitingDots(w io.Writer, message string) *WaitingDots {
	return &WaitingDots{w: w, message: message, done: make(chan struct{})}
}

// Start begins the waiting animation
func (d *WaitingDots) Start() {
	go func() {
		dots := 0
		fmt.Fprintf(d.w, "  %s", d.message)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-d.done:
				return
			case <-ticker.C:
				dots = (dots + 1) % 4
				fmt.Fprintf(d.w, "\r  %s%s", d.message, strings.Repeat(".", dots)+strings.Repeat(" ", 3-dots))
			}
		}
	}()
}

// Stop ends the waiting animation
func (d *WaitingDots) Stop(success bool) {
	close(d.done)
	if success {
		fmt.Fprintf(d.w, "\r  %s%s\n", d.message, SuccessStyle.Render(" done"))
	} else {
		fmt.Fprintf(d.w, "\r  %s%s\n", d.message, ErrorStyle.Render(" failed"))
	}
}
