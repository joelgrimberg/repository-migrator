package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sh(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func initRepoWithCommit(t *testing.T, root string) string {
	repo := filepath.Join(root, "src")
	os.MkdirAll(repo, 0o755)
	sh(t, repo, "init")
	// ensure branch is main
	sh(t, repo, "checkout", "-b", "main")
	sh(t, repo, "config", "user.email", "t@example.com")
	sh(t, repo, "config", "user.name", "Tester")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello"), 0o644)
	sh(t, repo, "add", ".")
	sh(t, repo, "commit", "-m", "init")
	return repo
}

func initBareRepo(t *testing.T, root string) string {
	bare := filepath.Join(root, "target.git")
	sh(t, root, "init", "--bare", bare)
	return bare
}

func TestAnalyzeFastForward_TargetEmpty_NewBranch(t *testing.T) {
	root := t.TempDir()
	src := initRepoWithCommit(t, root)
	tgt := initBareRepo(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	summary, ok, err := AnalyzeFastForward(ctx, src, tgt)
	if err != nil { t.Fatalf("AnalyzeFastForward: %v", err) }
	if !ok { t.Fatalf("expected fastForwardable=true") }
	if !strings.Contains(summary, "new=1") {
		t.Fatalf("expected new=1 in summary, got %q", summary)
	}
}

func TestAnalyzeFastForward_TargetUpToDate_FF(t *testing.T) {
	root := t.TempDir()
	src := initRepoWithCommit(t, root)
	tgt := initBareRepo(t, root)
	// push main to target so it matches source
	sh(t, src, "remote", "add", "t", tgt)
	sh(t, src, "push", "t", "main:main")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	summary, ok, err := AnalyzeFastForward(ctx, src, tgt)
	if err != nil { t.Fatalf("AnalyzeFastForward: %v", err) }
	if !ok { t.Fatalf("expected fastForwardable=true") }
	if !strings.Contains(summary, "ff=") { t.Fatalf("expected ff count in summary: %q", summary) }
}
