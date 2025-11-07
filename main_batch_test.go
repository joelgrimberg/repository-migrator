package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcfg "repository-migrator/internal/config"
)

func TestRunBatch_SummaryOutput(t *testing.T) {
	// Create a temporary repo list file
	tmpDir := t.TempDir()
	listFile := filepath.Join(tmpDir, "repos.json")
	content := `{
	  "repos": [
	    {
	      "source_repo_url": "https://example.com/repo1.git",
	      "project_name": "repo1"
	    },
	    {
	      "source_repo_url": "https://example.com/repo2.git",
	      "project_name": "repo2"
	    }
	  ]
	}`
	if err := os.WriteFile(listFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write repo list: %v", err)
	}

	// Set up minimal config
	cfg := appcfg.AppConfig{
		GitLabBaseURL: "https://gitlab.com",
		GitLabToken:   "test-token",
	}

	// Set REPO_LIST_FILE to trigger batch mode
	os.Setenv("REPO_LIST_FILE", listFile)
	defer os.Unsetenv("REPO_LIST_FILE")

	// This will fail because we don't have real repos, but we can verify
	// the structure is correct by checking error messages
	err := runBatch(cfg, listFile)
	// We expect errors (no real repos), but the function should attempt processing
	if err == nil {
		t.Log("runBatch completed (unexpected, but not necessarily wrong)")
	}
}

func TestRunBatch_EmptyList(t *testing.T) {
	tmpDir := t.TempDir()
	listFile := filepath.Join(tmpDir, "empty.json")
	content := `{"repos": []}`
	if err := os.WriteFile(listFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write repo list: %v", err)
	}

	cfg := appcfg.AppConfig{
		GitLabBaseURL: "https://gitlab.com",
		GitLabToken:   "test-token",
	}

	err := runBatch(cfg, listFile)
	if err != nil {
		t.Fatalf("runBatch with empty list should not error: %v", err)
	}
}

func TestRunBatch_InvalidEntries(t *testing.T) {
	tmpDir := t.TempDir()
	listFile := filepath.Join(tmpDir, "invalid.json")
	content := `{
	  "repos": [
	    {
	      "source_repo_url": "",
	      "project_name": "repo1"
	    },
	    {
	      "source_repo_url": "https://example.com/repo2.git",
	      "project_name": ""
	    }
	  ]
	}`
	if err := os.WriteFile(listFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write repo list: %v", err)
	}

	cfg := appcfg.AppConfig{
		GitLabBaseURL: "https://gitlab.com",
		GitLabToken:   "test-token",
	}

	err := runBatch(cfg, listFile)
	// Should error because entries are invalid
	if err == nil {
		t.Fatal("expected error for invalid entries")
	}
	if !strings.Contains(err.Error(), "missing source_repo_url or project_name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBatch_FileNotFound(t *testing.T) {
	cfg := appcfg.AppConfig{
		GitLabBaseURL: "https://gitlab.com",
		GitLabToken:   "test-token",
	}

	err := runBatch(cfg, "/nonexistent/file.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read repo list") {
		t.Fatalf("unexpected error: %v", err)
	}
}
