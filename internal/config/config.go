package config

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
)

type AppConfig struct {
    GitLabBaseURL string `json:"gitlab_base_url"`
    GitLabToken   string `json:"gitlab_token"`
    DefaultGroupPath string `json:"default_group_path,omitempty"`
}

func configDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("resolve home dir: %w", err)
    }
    return filepath.Join(home, ".config", "repository-migrator"), nil
}

func configPath() (string, error) {
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
    dir, err := configDir()
    if err != nil {
        return err
    }
    if err := os.MkdirAll(dir, 0o700); err != nil {
        return fmt.Errorf("mkdir config dir: %w", err)
    }
    p := filepath.Join(dir, "config.json")
    b, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal config: %w", err)
    }
    // Restrictive file permissions, token may be present
    tmp := p + ".tmp"
    if err := os.WriteFile(tmp, b, 0o600); err != nil {
        return fmt.Errorf("write temp config: %w", err)
    }
    if err := os.Rename(tmp, p); err != nil {
        return fmt.Errorf("persist config: %w", err)
    }
    return nil
}


