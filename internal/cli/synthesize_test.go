package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSynthesizeReadInputText_FileWithinLimit(t *testing.T) {
	cmd := NewSynthesizeCmd()
	cmd.text = ""
	cmd.maxInputBytes = 5

	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "input.txt")
	if err := os.WriteFile(inputFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp input file: %v", err)
	}
	cmd.file = inputFile

	got, err := cmd.readInputText()
	if err != nil {
		t.Fatalf("readInputText() unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("readInputText() = %q, want %q", got, "hello")
	}
}

func TestSynthesizeReadInputText_FileExceedsLimit(t *testing.T) {
	cmd := NewSynthesizeCmd()
	cmd.text = ""
	cmd.maxInputBytes = 4

	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "input.txt")
	if err := os.WriteFile(inputFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp input file: %v", err)
	}
	cmd.file = inputFile

	_, err := cmd.readInputText()
	if err == nil {
		t.Fatal("readInputText() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("readInputText() error = %T, want *UsageError", err)
	}
	if !strings.Contains(usageErr.Message, "input exceeds --max-input-bytes limit") {
		t.Fatalf("usage error message = %q, want limit message", usageErr.Message)
	}
}

func TestSynthesizeReadInputText_StdinExceedsLimit(t *testing.T) {
	cmd := NewSynthesizeCmd()
	cmd.text = ""
	cmd.file = "-"
	cmd.maxInputBytes = 3

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	defer r.Close()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	if _, err := w.Write([]byte("hello")); err != nil {
		w.Close()
		t.Fatalf("write stdin pipe: %v", err)
	}
	w.Close()

	_, err = cmd.readInputText()
	if err == nil {
		t.Fatal("readInputText() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("readInputText() error = %T, want *UsageError", err)
	}
}

func TestSynthesizeRun_RejectsNegativeMaxInputBytes(t *testing.T) {
	cmd := NewSynthesizeCmd()
	err := cmd.Run([]string{"--text", "hello", "--max-input-bytes", "-1"}, os.Stdout, os.Stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run() error = %T, want *UsageError", err)
	}
	if usageErr.Message != "--max-input-bytes must be >= 0" {
		t.Fatalf("usage error message = %q, want exact validation message", usageErr.Message)
	}
}
