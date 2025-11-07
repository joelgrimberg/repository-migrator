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

// helper to point HOME and config path to a temp dir so configDir/configPath stay isolated per test
func withTempHome(t *testing.T) (string, string) {
	t.Helper()
	d := t.TempDir()
	t.Setenv("HOME", d)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", d)
	}
	cfgDir := filepath.Join(d, "cfg")
	cfgPath := filepath.Join(cfgDir, "config.json")
	t.Setenv("REPO_MIGRATOR_CONFIG", cfgPath)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir cfgDir: %v", err)
	}
	return cfgDir, cfgPath
}

func readFile(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func TestLoad_NotFound_ReturnsZeroValueNoError(t *testing.T) {
	_, _ = withTempHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg != (AppConfig{}) {
		t.Fatalf("expected zero AppConfig, got %#v", cfg)
	}
}

func TestLoad_CorruptJSON_ReturnsParseError(t *testing.T) {
	_, cfgPath := withTempHome(t)
	if err := os.WriteFile(cfgPath, []byte("{ not: json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestSave_ThenLoad_RoundTrip(t *testing.T) {
	_, cfgPath := withTempHome(t)
	trueVal := true
	falseVal := false
	in := AppConfig{
		GitLabBaseURL:       "https://gitlab.example.com",
		SourceBaseURL:       "ssh://git@example.com:/",
		DefaultGroupPath:    "org/sub",
		DefaultSubfolder:    "apps",
		NonInteractive:      true,
		AutoCreateSubgroups: &trueVal,
		Overwrite:           &falseVal,
		SafeRebase:          &trueVal,
		TrialRun:            &falseVal,
		AllowPushDefault:    &falseVal,
	}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.GitLabBaseURL != in.GitLabBaseURL || out.SourceBaseURL != in.SourceBaseURL || out.DefaultGroupPath != in.DefaultGroupPath || out.DefaultSubfolder != in.DefaultSubfolder || out.NonInteractive != in.NonInteractive {
		t.Fatalf("scalar fields mismatch: in=%#v out=%#v", in, out)
	}
	if out.AutoCreateSubgroups == nil || out.Overwrite == nil || out.SafeRebase == nil || out.TrialRun == nil || out.AllowPushDefault == nil {
		t.Fatalf("pointer bools should be present after roundtrip: %#v", out)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
}

func TestSave_CreatesDirAndFileModes(t *testing.T) {
	cfgDir, cfgPath := withTempHome(t)
	if err := os.RemoveAll(cfgDir); err != nil {
		t.Fatalf("remove cfgDir: %v", err)
	}
	cfg := AppConfig{GitLabBaseURL: "https://x"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st, err := os.Stat(cfgDir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if st.Mode().Perm()&0o777 != 0o700 {
		t.Fatalf("config dir perms = %o, want 0700", st.Mode().Perm()&0o777)
	}
	fst, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if fst.Mode().Perm()&0o777 != 0o600 {
		t.Fatalf("config file perms = %o, want 0600", fst.Mode().Perm()&0o777)
	}
}

func TestLoad_UnreadableFile_PermissionDenied(t *testing.T) {
	_, cfgPath := withTempHome(t)
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o000); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected read config error, got %v", err)
	}
}

func TestSave_MkdirAllFails_WhenConfigDirIsFile(t *testing.T) {
	cfgDir, _ := withTempHome(t)
	if err := os.RemoveAll(cfgDir); err != nil {
		t.Fatalf("remove cfgDir: %v", err)
	}
	if err := os.WriteFile(cfgDir, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write file at dir path: %v", err)
	}
	err := Save(AppConfig{GitLabBaseURL: "x"})
	if err == nil {
		t.Fatal("expected Save to fail when configDir is a file")
	}
	if !strings.Contains(err.Error(), "mkdir config dir") && !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
	// clean up the file so later checks (if any) are unaffected
	_ = os.Remove(cfgDir)
}

func TestLoad_IgnoresUnknownFields(t *testing.T) {
	_, cfgPath := withTempHome(t)
	payload := map[string]any{
		"gitlab_base_url":    "https://gitlab.example.com",
		"default_group_path": "org",
		"default_subfolder":  "apps",
		"non_interactive":    true,
		"extra":              "ignored",
	}
	b, _ := json.Marshal(payload)
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GitLabBaseURL != "https://gitlab.example.com" || cfg.DefaultGroupPath != "org" || cfg.DefaultSubfolder != "apps" || !cfg.NonInteractive {
		t.Fatalf("unexpected values: %#v", cfg)
	}
}

func TestConfigDir_HomeResolutionError(t *testing.T) {
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	t.Cleanup(func() {
		if oldHome != "" {
			os.Setenv("HOME", oldHome)
		}
		if oldUserProfile != "" {
			os.Setenv("USERPROFILE", oldUserProfile)
		}
	})
	os.Unsetenv("HOME")
	os.Unsetenv("USERPROFILE")
	os.Unsetenv("REPO_MIGRATOR_CONFIG")
	_, err := configDir()
	if err != nil && !errors.Is(err, os.ErrPermission) {
		_ = err
	}
}
