package ui

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRunWithSpinnerFallsBackWithoutTerminal(t *testing.T) {
	var out bytes.Buffer
	called := false

	if err := RunWithSpinner(&out, "Doing work", func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("RunWithSpinner() error: %v", err)
	}

	if !called {
		t.Fatal("RunWithSpinner() did not run function")
	}
	if got := out.String(); !strings.Contains(got, "Doing work") || !strings.Contains(got, "✓") {
		t.Fatalf("output = %q, want plain non-terminal progress", got)
	}
}

func TestRunWithSpinnerFallbackReturnsError(t *testing.T) {
	var out bytes.Buffer
	wantErr := errors.New("boom")

	err := RunWithSpinner(&out, "Doing work", func() error {
		return wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("RunWithSpinner() error = %v, want %v", err, wantErr)
	}
	if got := out.String(); !strings.Contains(got, "✗") {
		t.Fatalf("output = %q, want failure marker", got)
	}
}

func TestSpinnerModelReturnsDoneError(t *testing.T) {
	wantErr := errors.New("boom")
	model, _ := NewSpinner("Doing work").Update(doneMsg{err: wantErr})

	got := model.(SpinnerModel)
	if !errors.Is(got.err, wantErr) {
		t.Fatalf("SpinnerModel error = %v, want %v", got.err, wantErr)
	}
	if gotView := got.View(); !strings.Contains(gotView, "✗") {
		t.Fatalf("View() = %q, want failure marker", gotView)
	}
}

func TestSpinnerModelCtrlCReturnsInterrupted(t *testing.T) {
	model, _ := NewSpinner("Doing work").Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	got := model.(SpinnerModel)
	if got.err == nil || !strings.Contains(got.err.Error(), "interrupted") {
		t.Fatalf("SpinnerModel error = %v, want interrupted", got.err)
	}
}

func TestRunWithSpinnerInteractiveReturnsFunctionError(t *testing.T) {
	if !hasInteractiveTerminal(os.Stdout) {
		t.Skip("requires interactive terminal")
	}

	wantErr := errors.New("boom")
	err := RunWithSpinner(os.Stdout, "Doing work", func() error {
		return wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("RunWithSpinner() error = %v, want %v", err, wantErr)
	}
}

func TestRunWithWaitingFallsBackWithoutTerminal(t *testing.T) {
	var out bytes.Buffer
	attempts := 0

	err := RunWithWaiting(&out, "Waiting", time.Nanosecond, func() (bool, error) {
		attempts++
		return attempts == 2, nil
	})

	if err != nil {
		t.Fatalf("RunWithWaiting() error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if got := out.String(); !strings.Contains(got, "Waiting") || !strings.Contains(got, "✓") {
		t.Fatalf("output = %q, want plain non-terminal progress", got)
	}
}
