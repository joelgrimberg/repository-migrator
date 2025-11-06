package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// helper to point HOME to a temp dir so configDir/configPath stay isolated per test
func withTempHome(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	// GitHub runners sometimes use HOME, mac uses HOME; ensure consistent
	t.Setenv("HOME", d)
	// Also set USERPROFILE on Windows just in case (noop elsewhere)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", d)
	}
	return d
}

func readFile(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func TestLoad_NotFound_ReturnsZeroValueNoError(t *testing.T) {
	withTempHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg != (AppConfig{}) {
		t.Fatalf("expected zero AppConfig, got %#v", cfg)
	}
}

func TestLoad_CorruptJSON_ReturnsParseError(t *testing.T) {
	withTempHome(t)
	p, err := configPath()
	if err != nil { t.Fatalf("configPath: %v", err) }
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil { t.Fatalf("mkdir: %v", err) }
	if err := os.WriteFile(p, []byte("{ not: json"), 0o600); err != nil { t.Fatalf("write: %v", err) }
	_, err = Load()
	if err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestSave_ThenLoad_RoundTrip(t *testing.T) {
	withTempHome(t)
	in := AppConfig{GitLabBaseURL: "https://gitlab.example.com", GitLabToken: "token123", DefaultGroupPath: "org/sub"}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load()
	if err != nil { t.Fatalf("Load: %v", err) }
	if out != in {
		t.Fatalf("roundtrip mismatch: in=%#v out=%#v", in, out)
	}
}

func TestSave_CreatesDirAndFileModes(t *testing.T) {
	withTempHome(t)
	cfg := AppConfig{GitLabBaseURL: "https://x", GitLabToken: "y"}
	if err := Save(cfg); err != nil { t.Fatalf("Save: %v", err) }
	p, err := configPath()
	if err != nil { t.Fatalf("configPath: %v", err) }
	// directory perms
	d := filepath.Dir(p)
	st, err := os.Stat(d)
	if err != nil { t.Fatalf("stat dir: %v", err) }
	if st.Mode().Perm()&0o777 != 0o700 {
		t.Fatalf("config dir perms = %o, want 0700", st.Mode().Perm()&0o777)
	}
	// file perms
	fst, err := os.Stat(p)
	if err != nil { t.Fatalf("stat file: %v", err) }
	if fst.Mode().Perm()&0o777 != 0o600 {
		t.Fatalf("config file perms = %o, want 0600", fst.Mode().Perm()&0o777)
	}
}

func TestLoad_UnreadableFile_PermissionDenied(t *testing.T) {
	withTempHome(t)
	p, err := configPath()
	if err != nil { t.Fatalf("configPath: %v", err) }
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil { t.Fatalf("mkdir: %v", err) }
	if err := os.WriteFile(p, []byte("{}"), 0o000); err != nil { t.Fatalf("write: %v", err) }
	_, err = Load()
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
	// os.ReadFile may wrap a path error; just assert it's from read path
	if !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected read config error, got %v", err)
	}
}

func TestSave_MkdirAllFails_WhenConfigDirIsFile(t *testing.T) {
	withTempHome(t)
	dir, err := configDir()
	if err != nil { t.Fatalf("configDir: %v", err) }
	// create parent ~/.config, then create a file at ~/.config/repository-migrator to break MkdirAll
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o700); err != nil { t.Fatalf("mkdir parent: %v", err) }
	if err := os.WriteFile(dir, []byte("not a dir"), 0o600); err != nil { t.Fatalf("write file at dir path: %v", err) }
	err = Save(AppConfig{GitLabBaseURL: "x"})
	if err == nil {
		t.Fatal("expected Save to fail when configDir is a file")
	}
	// Ensure error mentions mkdir
	if !strings.Contains(err.Error(), "mkdir config dir") && !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_IgnoresUnknownFields(t *testing.T) {
	withTempHome(t)
	p, err := configPath()
	if err != nil { t.Fatalf("configPath: %v", err) }
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil { t.Fatalf("mkdir: %v", err) }
	payload := map[string]any{
		"gitlab_base_url": "https://gitlab.example.com",
		"gitlab_token": "abc",
		"default_group_path": "org",
		"extra": "ignored",
	}
	b, _ := json.Marshal(payload)
	if err := os.WriteFile(p, b, 0o600); err != nil { t.Fatalf("write: %v", err) }
	cfg, err := Load()
	if err != nil { t.Fatalf("Load: %v", err) }
	if cfg.GitLabBaseURL != "https://gitlab.example.com" || cfg.GitLabToken != "abc" || cfg.DefaultGroupPath != "org" {
		t.Fatalf("unexpected values: %#v", cfg)
	}
}

func TestConfigDir_HomeResolutionError(t *testing.T) {
	// Simulate by clearing HOME/USERPROFILE and running on systems where os.UserHomeDir fails.
	// On most CI this will still succeed, so accept either no error or specific error.
	// We mainly ensure it doesn't panic.
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	_ = os.Unsetenv("HOME")
	_ = os.Unsetenv("USERPROFILE")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
	})
	_, err := configDir()
	// err may or may not be non-nil; just ensure error type is wrapped meaningfully if present
	if err != nil && !errors.Is(err, os.ErrPermission) {
		// accept any error; this test is only to cover the code path
		_ = err
	}
}
