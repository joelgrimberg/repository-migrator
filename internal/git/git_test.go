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

// shOut runs git and returns trimmed stdout (failing the test on error).
func shOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

// commitFile writes a file with content, stages it, and commits with the given message.
func commitFile(t *testing.T, repo, path, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	sh(t, repo, "add", ".")
	sh(t, repo, "commit", "-m", msg)
}

// TestSafeRebaseAndPush_TagConflict reproduces the production scenario where
// source and target both have a tag with the same name pointing to different
// commits. The migration must NOT clobber either side and must surface the
// conflict, while still pushing source-only tags.
func TestSafeRebaseAndPush_TagConflict(t *testing.T) {
	root := t.TempDir()
	src := initRepoWithCommit(t, root) // creates "src" with main + a.txt
	tgt := initBareRepo(t, root)       // creates target.git

	// Source: commit A on main, tag 7.2.0.X -> A, tag v6.1.4 -> separate commit C
	srcMainSHA := shOut(t, src, "rev-parse", "HEAD")
	sh(t, src, "tag", "7.2.0.X", srcMainSHA)
	commitFile(t, src, "b.txt", "src-only", "feat: another commit on source")
	srcCommitC := shOut(t, src, "rev-parse", "HEAD")
	sh(t, src, "tag", "v6.1.4", srcCommitC)

	// Target: push main first so branch exists, then create a divergent commit
	// reachable only from target's 7.2.0.X tag, plus a target-only tag v9.9.9.
	sh(t, src, "remote", "add", "tgtpush", tgt)
	sh(t, src, "push", "tgtpush", "main:main")

	// Build a divergent target state via a separate working clone.
	targetWork := filepath.Join(root, "target-work")
	sh(t, root, "clone", tgt, targetWork)
	sh(t, targetWork, "config", "user.email", "t@example.com")
	sh(t, targetWork, "config", "user.name", "Tester")
	sh(t, targetWork, "checkout", "main")
	commitFile(t, targetWork, "target-only.txt", "target-only", "hotfix: prod incident")
	targetDivergentSHA := shOut(t, targetWork, "rev-parse", "HEAD")
	sh(t, targetWork, "tag", "7.2.0.X", targetDivergentSHA)
	// also a target-only tag that should be untouched
	sh(t, targetWork, "tag", "v9.9.9", targetDivergentSHA)
	// push the divergent main (so the branch on target is ahead of source by one commit on top of common base)
	// Note: source main is just commit A; target main will be A + hotfix.
	// That makes branches "ff" from target's perspective (target is ahead of source),
	// i.e. branchRelation returns "only-target" for main -> no push attempted, fine for this test.
	sh(t, targetWork, "push", "origin", "main:main")
	sh(t, targetWork, "push", "origin", "7.2.0.X")
	sh(t, targetWork, "push", "origin", "v9.9.9")

	// Capture target's tag SHAs before migration (should remain unchanged).
	tgtTagBefore := shOut(t, targetWork, "rev-parse", "7.2.0.X")
	tgtV999Before := shOut(t, targetWork, "rev-parse", "v9.9.9")

	// Run migration
	workDir := filepath.Join(root, "workdir")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conflicts, err := SafeRebaseAndPush(ctx, src, tgt, workDir)

	// Expect a conflict on 7.2.0.X and an error describing it.
	if err == nil {
		t.Fatalf("expected error due to tag conflict, got nil")
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(conflicts), conflicts)
	}
	if conflicts[0].Name != "7.2.0.X" {
		t.Fatalf("expected conflict on 7.2.0.X, got %s", conflicts[0].Name)
	}
	if conflicts[0].SourceSHA != srcMainSHA || conflicts[0].TargetSHA != targetDivergentSHA {
		t.Fatalf("conflict SHAs wrong: src=%s tgt=%s (want src=%s tgt=%s)",
			conflicts[0].SourceSHA, conflicts[0].TargetSHA, srcMainSHA, targetDivergentSHA)
	}

	// Verify target's 7.2.0.X tag was NOT clobbered.
	sh(t, targetWork, "fetch", "--tags", "-f", "origin")
	tgtTagAfter := shOut(t, targetWork, "rev-parse", "7.2.0.X")
	if tgtTagAfter != tgtTagBefore {
		t.Fatalf("target tag 7.2.0.X was clobbered: before=%s after=%s", tgtTagBefore, tgtTagAfter)
	}

	// Verify v6.1.4 (source-only) WAS pushed to target.
	v614 := shOut(t, targetWork, "rev-parse", "v6.1.4")
	if v614 != srcCommitC {
		t.Fatalf("v6.1.4 missing or wrong on target: got=%s want=%s", v614, srcCommitC)
	}

	// Verify v9.9.9 (target-only) untouched.
	tgtV999After := shOut(t, targetWork, "rev-parse", "v9.9.9")
	if tgtV999After != tgtV999Before {
		t.Fatalf("target-only tag v9.9.9 changed: before=%s after=%s", tgtV999Before, tgtV999After)
	}
}

// TestSafeRebaseAndPush_TagsNoConflict verifies the happy path: source tags
// that don't exist on target are pushed, and the function returns no error.
func TestSafeRebaseAndPush_TagsNoConflict(t *testing.T) {
	root := t.TempDir()
	src := initRepoWithCommit(t, root)
	tgt := initBareRepo(t, root)

	// tag source main as v1.0.0; add another commit and tag it v1.1.0
	srcMainSHA := shOut(t, src, "rev-parse", "HEAD")
	sh(t, src, "tag", "v1.0.0", srcMainSHA)
	commitFile(t, src, "b.txt", "more", "feat: more")
	srcSecond := shOut(t, src, "rev-parse", "HEAD")
	sh(t, src, "tag", "v1.1.0", srcSecond)

	// push main to target so the branch exists on both sides (no tags pushed yet)
	sh(t, src, "remote", "add", "tgtpush", tgt)
	sh(t, src, "push", "tgtpush", "main:main")

	workDir := filepath.Join(root, "workdir")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conflicts, err := SafeRebaseAndPush(ctx, src, tgt, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %+v", conflicts)
	}

	// Verify both tags pushed
	targetCheck := filepath.Join(root, "target-check")
	sh(t, root, "clone", tgt, targetCheck)
	v100 := shOut(t, targetCheck, "rev-parse", "v1.0.0")
	if v100 != srcMainSHA {
		t.Fatalf("v1.0.0 wrong on target: got=%s want=%s", v100, srcMainSHA)
	}
	v110 := shOut(t, targetCheck, "rev-parse", "v1.1.0")
	if v110 != srcSecond {
		t.Fatalf("v1.1.0 wrong on target: got=%s want=%s", v110, srcSecond)
	}
}

// seedTargetFromSource pushes the source repo into a bare target so the target
// has source's history on main. Mirrors what a mirror-push step does in the
// migration pipeline before the overlay runs.
func seedTargetFromSource(t *testing.T, src, tgt string) {
	t.Helper()
	sh(t, src, "remote", "add", "seedtgt", tgt)
	sh(t, src, "push", "seedtgt", "main:main")
}

func TestApplySecurityCIOverlay_FreshTarget(t *testing.T) {
	root := t.TempDir()
	src := initRepoWithCommit(t, root)
	tgt := initBareRepo(t, root)
	seedTargetFromSource(t, src, tgt)

	template := []byte("include:\n  - template: Security/SAST.gitlab-ci.yml\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	overwrote, err := ApplySecurityCIOverlay(ctx, tgt, "main", template, "Repository Migrator", "migrator@local")
	if err != nil {
		t.Fatalf("ApplySecurityCIOverlay: %v", err)
	}
	if overwrote {
		t.Fatalf("expected overwroteSource=false for a target without .gitlab-ci.yml")
	}

	// Clone target into a fresh check dir and verify file + commit metadata.
	check := filepath.Join(root, "check")
	sh(t, root, "clone", "--branch", "main", tgt, check)
	content, err := os.ReadFile(filepath.Join(check, ".gitlab-ci.yml"))
	if err != nil {
		t.Fatalf("read .gitlab-ci.yml on target: %v", err)
	}
	if string(content) != string(template) {
		t.Fatalf(".gitlab-ci.yml content mismatch:\n got: %q\nwant: %q", string(content), string(template))
	}

	subject := shOut(t, check, "log", "-1", "--pretty=%s")
	if !strings.Contains(subject, "security pipeline") {
		t.Fatalf("expected overlay commit subject to mention security pipeline, got %q", subject)
	}
	author := shOut(t, check, "log", "-1", "--pretty=%an <%ae>")
	if author != "Repository Migrator <migrator@local>" {
		t.Fatalf("unexpected author: %q", author)
	}
}

func TestApplySecurityCIOverlay_SourceHadCI(t *testing.T) {
	root := t.TempDir()
	src := initRepoWithCommit(t, root)
	// source already has its own .gitlab-ci.yml
	commitFile(t, src, ".gitlab-ci.yml", "stages: [build]\nbuild: { script: 'echo source' }\n", "ci: source pipeline")
	tgt := initBareRepo(t, root)
	seedTargetFromSource(t, src, tgt)

	template := []byte("include:\n  - template: Security/SAST.gitlab-ci.yml\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	overwrote, err := ApplySecurityCIOverlay(ctx, tgt, "main", template, "Repository Migrator", "migrator@local")
	if err != nil {
		t.Fatalf("ApplySecurityCIOverlay: %v", err)
	}
	if !overwrote {
		t.Fatalf("expected overwroteSource=true when source had a .gitlab-ci.yml")
	}

	// Verify the bundled template wins on target.
	check := filepath.Join(root, "check")
	sh(t, root, "clone", "--branch", "main", tgt, check)
	content, err := os.ReadFile(filepath.Join(check, ".gitlab-ci.yml"))
	if err != nil {
		t.Fatalf("read .gitlab-ci.yml on target: %v", err)
	}
	if string(content) != string(template) {
		t.Fatalf("bundled template did not win on target: got %q", string(content))
	}
}

// TestApplySecurityCIOverlay_AfterMirrorReset simulates a re-sync: the
// pipeline force-pushes source over target (wiping the previous overlay's
// CI commit), then runs the overlay again. Each sync should produce a fresh
// CI commit on top of the just-mirrored source HEAD.
func TestApplySecurityCIOverlay_AfterMirrorReset(t *testing.T) {
	root := t.TempDir()
	src := initRepoWithCommit(t, root)
	tgt := initBareRepo(t, root)
	seedTargetFromSource(t, src, tgt)

	template := []byte("include:\n  - template: Security/SAST.gitlab-ci.yml\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First sync: mirror already done above; apply overlay.
	if _, err := ApplySecurityCIOverlay(ctx, tgt, "main", template, "Repository Migrator", "migrator@local"); err != nil {
		t.Fatalf("first overlay: %v", err)
	}

	// Simulate the next sync's mirror push: source advances, then force-push
	// source main over target main (wiping the overlay commit from last sync).
	commitFile(t, src, "c.txt", "next", "feat: next source commit")
	sh(t, src, "push", "--force", "seedtgt", "main:main")

	// Second sync's overlay: file is no longer on target (mirror wiped it),
	// overlay reads it as fresh and commits.
	overwrote, err := ApplySecurityCIOverlay(ctx, tgt, "main", template, "Repository Migrator", "migrator@local")
	if err != nil {
		t.Fatalf("second overlay: %v", err)
	}
	if overwrote {
		t.Fatalf("expected overwroteSource=false after mirror reset wiped the prior overlay")
	}

	// Target main HEAD should now be: source-c-commit + overlay commit.
	check := filepath.Join(root, "check")
	sh(t, root, "clone", "--branch", "main", tgt, check)
	subject := shOut(t, check, "log", "-1", "--pretty=%s")
	if !strings.Contains(subject, "security pipeline") {
		t.Fatalf("expected overlay commit at HEAD after second sync, got %q", subject)
	}
	parentSubject := shOut(t, check, "log", "-1", "--pretty=%s", "HEAD~1")
	if !strings.Contains(parentSubject, "next source commit") {
		t.Fatalf("expected HEAD~1 to be the new source commit, got %q", parentSubject)
	}
}
