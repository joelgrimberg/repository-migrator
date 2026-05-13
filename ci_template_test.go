package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCITemplate_DefaultUsesEmbedded(t *testing.T) {
	t.Setenv("CI_TEMPLATE_FILE", "")
	*ciTemplateFlag = ""

	content, source, err := resolveCITemplate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != embeddedTemplateSource {
		t.Fatalf("source label: got %q want %q", source, embeddedTemplateSource)
	}
	if len(content) == 0 {
		t.Fatalf("embedded template is empty")
	}
	if string(content) != string(ciTemplate) {
		t.Fatalf("content does not match embedded ciTemplate")
	}
}

func TestResolveCITemplate_EnvVarPathWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "security-pipeline.yml")
	want := []byte("include:\n  - template: Security/SAST.gitlab-ci.yml\n# operator-managed\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	t.Setenv("CI_TEMPLATE_FILE", path)
	*ciTemplateFlag = ""

	content, source, err := resolveCITemplate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != path {
		t.Fatalf("source: got %q want %q", source, path)
	}
	if string(content) != string(want) {
		t.Fatalf("content mismatch:\n got %q\nwant %q", string(content), string(want))
	}
}

func TestResolveCITemplate_EnvVarMissingPathErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yml")
	t.Setenv("CI_TEMPLATE_FILE", missing)
	*ciTemplateFlag = ""

	_, _, err := resolveCITemplate()
	if err == nil {
		t.Fatalf("expected error for missing CI_TEMPLATE_FILE path, got nil")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("error should mention the missing path %q; got: %v", missing, err)
	}
}

func TestResolveCITemplate_FlagPathUsedWhenEnvEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "from-flag.yml")
	want := []byte("# flag-provided template\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	t.Setenv("CI_TEMPLATE_FILE", "")
	prev := *ciTemplateFlag
	*ciTemplateFlag = path
	t.Cleanup(func() { *ciTemplateFlag = prev })

	content, source, err := resolveCITemplate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != path {
		t.Fatalf("source: got %q want %q", source, path)
	}
	if string(content) != string(want) {
		t.Fatalf("content mismatch")
	}
}
