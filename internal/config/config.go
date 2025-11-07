package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	GitLabBaseURL       string `json:"gitlab_base_url"`
	SourceBaseURL       string `json:"source_base_url,omitempty"`
	DefaultGroupPath    string `json:"default_group_path,omitempty"`
	DefaultSubfolder    string `json:"default_subfolder,omitempty"`
	NonInteractive      bool   `json:"non_interactive,omitempty"`
	AutoCreateSubgroups *bool  `json:"auto_create_subgroups,omitempty"`
	Overwrite           *bool  `json:"overwrite,omitempty"`
	SafeRebase          *bool  `json:"safe_rebase,omitempty"`
	TrialRun            *bool  `json:"trial_run,omitempty"`
	AllowPushDefault    *bool  `json:"allow_push_default_branch,omitempty"`
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "repository-migrator"), nil
}

func repoConfigDefault() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "repository-migrator.config.json"), nil
}

func configPath() (string, error) {
	if envPath := strings.TrimSpace(os.Getenv("REPO_MIGRATOR_CONFIG")); envPath != "" {
		if !filepath.IsAbs(envPath) {
			wd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			envPath = filepath.Join(wd, envPath)
		}
		return envPath, nil
	}

	if wd, err := os.Getwd(); err == nil {
		// Prefer repository-local config if present
		candidate := filepath.Join(wd, "repository-migrator.config.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		legacy := filepath.Join(wd, "config.json")
		if _, err := os.Stat(legacy); err == nil {
			return legacy, nil
		}
		// Default to repository-migrator.config.json in cwd even if it does not exist yet
		return candidate, nil
	}

	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (AppConfig, error) {
	var cfg AppConfig
	p, err := configPath()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func Save(cfg AppConfig) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("persist config: %w", err)
	}
	return nil
}

// ResolveConfigPath returns the path that Load/Save use for the configuration file.
// It can be used by other packages (e.g. logs) to co-locate artifacts with the config.
func ResolveConfigPath() (string, error) {
	return configPath()
}
