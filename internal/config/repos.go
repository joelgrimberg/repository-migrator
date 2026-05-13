package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type RepoEntry struct {
	SourceRepoURL    string `json:"source_repo_url,omitempty"`
	SourceName       string `json:"source_name,omitempty"`
	ProjectName      string `json:"project_name"`
	GroupPath        string `json:"group_path,omitempty"`
	Subfolder        string `json:"subfolder,omitempty"`
	Overwrite        *bool  `json:"overwrite,omitempty"`
	SafeRebase       *bool  `json:"safe_rebase,omitempty"`
	TrialRun         *bool  `json:"trial_run,omitempty"`
	AllowPushDefault *bool  `json:"allow_push_default_branch,omitempty"`
	AddSecurityCI    *bool  `json:"add_security_ci,omitempty"`
}

type RepoList struct {
	Repos []RepoEntry `json:"repos"`
}

func LoadRepoList(path string) (RepoList, error) {
	var rl RepoList
	b, err := os.ReadFile(path)
	if err != nil {
		return rl, fmt.Errorf("read repo list: %w", err)
	}
	if err := json.Unmarshal(b, &rl); err != nil {
		return rl, fmt.Errorf("parse repo list: %w", err)
	}
	return rl, nil
}
