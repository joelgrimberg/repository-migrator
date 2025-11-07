package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

func CloneMirror(ctx context.Context, srcURL string, destDir string) error {
	return run(ctx, "", "clone", "--mirror", srcURL, destDir)
}

func PushMirror(ctx context.Context, repoDir string, remoteURL string) error {
	// Using --prune to remove refs that don't exist on source
	return run(ctx, repoDir, "push", "--mirror", "--prune", remoteURL)
}

// CheckRemote verifies that the given remote URL is accessible by running git ls-remote.
func CheckRemote(ctx context.Context, remoteURL string) error {
	return run(ctx, "", "ls-remote", remoteURL, "HEAD")
}
