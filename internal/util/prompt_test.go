package util

import (
	"io"
	"os"
	"strings"
	"testing"
)

func withStdin(t *testing.T, s string) func() {
	r, w, err := os.Pipe()
	if err != nil { t.Fatalf("pipe: %v", err) }
	old := os.Stdin
	os.Stdin = r
	_, _ = io.WriteString(w, s)
	_ = w.Close()
	return func() { os.Stdin = old }
}

func TestPrompt_ReadsLineAndTrims(t *testing.T) {
	cleanup := withStdin(t, "  hello world  \n")
	defer cleanup()
	out, err := Prompt("Enter: ")
	if err != nil { t.Fatalf("Prompt error: %v", err) }
	if out != "hello world" {
		t.Fatalf("got %q want %q", out, "hello world")
	}
}

func TestPromptOptional_Passthrough(t *testing.T) {
	cleanup := withStdin(t, "value\n")
	defer cleanup()
	out, err := PromptOptional("Opt: ")
	if err != nil { t.Fatalf("PromptOptional error: %v", err) }
	if out != "value" { t.Fatalf("got %q", out) }
}

func TestPromptWithDefault_UsesDefaultOnEmpty(t *testing.T) {
	cleanup := withStdin(t, "   \n")
	defer cleanup()
	out, err := PromptWithDefault("Label:", "DEF")
	if err != nil { t.Fatalf("PromptWithDefault error: %v", err) }
	if out != "DEF" { t.Fatalf("got %q want DEF", out) }
}

func TestPromptWithDefault_UsesInputWhenProvided(t *testing.T) {
	cleanup := withStdin(t, "abc\n")
	defer cleanup()
	out, err := PromptWithDefault("Label:", "DEF")
	if err != nil { t.Fatalf("PromptWithDefault error: %v", err) }
	if out != "abc" { t.Fatalf("got %q want abc", out) }
}

func TestPromptWithDefault_LabelFormatting(t *testing.T) {
	// Ensure label formatting doesn't affect returned value; can't capture stdout here easily,
	// but we can assert that non-empty input bypasses default logic.
	cleanup := withStdin(t, strings.Repeat(" ", 1)+"x\n")
	defer cleanup()
	out, err := PromptWithDefault("Some label:", "DEF")
	if err != nil { t.Fatalf("PromptWithDefault error: %v", err) }
	if out != "x" { t.Fatalf("got %q want x", out) }
}
