package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const ciOverlayFilename = ".gitlab-ci.yml"

// ApplySecurityCIOverlay clones targetURL into a temp working tree, writes
// ciContent to .gitlab-ci.yml on the given branch, commits, and pushes.
// Returns overwroteSource=true only when the file existed with content
// genuinely different from ciContent and that content was actually
// replaced by a new commit (i.e. source had its own distinct .gitlab-ci.yml
// that was just mirrored to target). It returns false whenever nothing was
// actually overwritten — the file was new, or its content already matched
// ciContent (e.g. a repeat run once steady state is reached) — so a caller
// warning about replacement doesn't fire on every idempotent re-run. The
// push is a normal fast-forward — no force needed — because the local
// commit is a direct child of target's current branch tip.
func ApplySecurityCIOverlay(
	ctx context.Context,
	targetURL, branch string,
	ciContent []byte,
	authorName, authorEmail string,
) (overwroteSource bool, err error) {
	tmpDir, err := os.MkdirTemp("", "ci-overlay-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmpDir)

	workDir := filepath.Join(tmpDir, "repo")
	if err := run(ctx, "", "clone", "--branch", branch, "--single-branch", targetURL, workDir); err != nil {
		return false, fmt.Errorf("clone target for CI overlay: %w", err)
	}

	if err := run(ctx, workDir, "config", "user.name", authorName); err != nil {
		return false, err
	}
	if err := run(ctx, workDir, "config", "user.email", authorEmail); err != nil {
		return false, err
	}

	ciPath := filepath.Join(workDir, ciOverlayFilename)
	existing, readErr := os.ReadFile(ciPath)
	hadDifferentContent := readErr == nil && !bytes.Equal(existing, ciContent)

	if err := os.WriteFile(ciPath, ciContent, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", ciOverlayFilename, err)
	}

	if err := run(ctx, workDir, "add", ciOverlayFilename); err != nil {
		return false, err
	}

	staged, err := hasStagedChanges(ctx, workDir)
	if err != nil {
		return false, err
	}
	if !staged {
		// Target's .gitlab-ci.yml already matches ciContent (e.g. source
		// carries the same template directly) — nothing to commit, push,
		// or report as overwritten.
		return false, nil
	}

	if err := run(ctx, workDir, "commit", "-m", "chore(ci): apply security pipeline"); err != nil {
		return false, err
	}
	if err := run(ctx, workDir, "push", "origin", branch); err != nil {
		return false, fmt.Errorf("push CI overlay: %w", err)
	}
	return hadDifferentContent, nil
}

// hasStagedChanges reports whether workDir's index differs from HEAD.
func hasStagedChanges(ctx context.Context, workDir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	cmd.Dir = workDir
	err := cmd.Run()
	if err == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("git diff --cached --quiet: %w", err)
}
