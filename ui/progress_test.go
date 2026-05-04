package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
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
