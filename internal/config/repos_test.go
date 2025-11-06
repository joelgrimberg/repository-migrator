package config

import (
	"os"
	"testing"
)

func TestLoadRepoList_OK(t *testing.T) {
	f := t.TempDir() + "/list.json"
	content := `{
	  "repos": [
	    {
	      "source_repo_url": "https://example.com/a.git",
	      "project_name": "a",
	      "group_path": "org/sub",
	      "trial_run": true
	    },
	    {
	      "source_repo_url": "git@example.com:b/c.git",
	      "project_name": "c",
	      "overwrite": true
	    }
	  ]
	}`
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil { t.Fatal(err) }
	rl, err := LoadRepoList(f)
	if err != nil { t.Fatalf("LoadRepoList: %v", err) }
	if len(rl.Repos) != 2 { t.Fatalf("expected 2 repos, got %d", len(rl.Repos)) }
	if rl.Repos[0].ProjectName != "a" || rl.Repos[1].ProjectName != "c" {
		t.Fatalf("unexpected names: %#v", rl)
	}
	if rl.Repos[0].TrialRun == nil || !*rl.Repos[0].TrialRun {
		t.Fatalf("expected trial_run=true for first entry")
	}
	if rl.Repos[1].Overwrite == nil || !*rl.Repos[1].Overwrite {
		t.Fatalf("expected overwrite=true for second entry")
	}
}

func TestLoadRepoList_FileMissing(t *testing.T) {
	_, err := LoadRepoList(t.TempDir() + "/missing.json")
	if err == nil { t.Fatal("expected error for missing file") }
}

func TestLoadRepoList_BadJSON(t *testing.T) {
	f := t.TempDir() + "/bad.json"
	if err := os.WriteFile(f, []byte("{"), 0o600); err != nil { t.Fatal(err) }
	_, err := LoadRepoList(f)
	if err == nil { t.Fatal("expected parse error") }
}
