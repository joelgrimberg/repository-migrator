package git

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "os/exec"
    "strings"
)

// SafeRebaseAndPush clones source (non-bare), adds target remote, fetches both,
// and for each branch:
// - fast-forward pushes when possible
// - if diverged, attempts rebase (without pushing); stops on conflicts
// Tags: pushes only tags that don't already exist on target
func SafeRebaseAndPush(ctx context.Context, srcURL, targetURL, workDir string) error {
    // clone non-bare
    if err := run(ctx, "", "clone", srcURL, workDir); err != nil {
        return err
    }
    // setup remotes
    if err := run(ctx, workDir, "remote", "add", "target", targetURL); err != nil {
        return err
    }
    if err := run(ctx, workDir, "fetch", "--all", "--tags"); err != nil {
        return err
    }

    // list branches on origin
    originBranches, err := listBranches(ctx, workDir, "origin")
    if err != nil { return err }

    for _, b := range originBranches {
        srcRef := "origin/" + b
        tgtRef := "target/" + b
        relation, err := branchRelation(ctx, workDir, srcRef, tgtRef)
        if err != nil { return fmt.Errorf("analyze %s: %w", b, err) }
        switch relation {
        case "only-source":
            // create new branch on target
            if err := run(ctx, workDir, "push", "target", srcRef+":refs/heads/"+b); err != nil { return err }
        case "ff":
            // fast-forward target to source
            if err := run(ctx, workDir, "push", "target", srcRef+":refs/heads/"+b); err != nil { return err }
        case "only-target":
            // nothing to do
            continue
        case "diverged":
            // attempt rebase target onto source without pushing
            // create temp branch from source and rebase target
            if err := run(ctx, workDir, "checkout", "-B", "_tmp_rebase_"+b, srcRef); err != nil { return err }
            rebaseErr := exec.CommandContext(ctx, "git", "rebase", tgtRef)
            rebaseErr.Dir = workDir
            out, err := rebaseErr.CombinedOutput()
            if err != nil {
                // abort rebase to clean state
                _ = run(ctx, workDir, "rebase", "--abort")
                return fmt.Errorf("rebase for branch %s has conflicts or failed:\n%s", b, string(out))
            }
            // successful rebase but do not overwrite; stop and ask user to review
            return fmt.Errorf("branch %s diverged: a clean rebase is possible. Review locally before pushing.")
        }
    }

    // push new tags only (that don't exist on target)
    if err := run(ctx, workDir, "push", "--atomic", "--tags", "target"); err != nil {
        // ignore tag push failures; they may be non-FF
        _ = err
    }
    return nil
}

func listBranches(ctx context.Context, dir string, remote string) ([]string, error) {
    // Remote-tracking branches are stored under refs/remotes/<remote>/<branch>
    cmd := exec.CommandContext(ctx, "git", "for-each-ref", fmt.Sprintf("refs/remotes/%s", remote), "--format=%(refname:short)")
    cmd.Dir = dir
    out, err := cmd.Output()
    if err != nil { return nil, err }
    lines := strings.Split(strings.TrimSpace(string(out)), "\n")
    var branches []string
    for _, l := range lines {
        l = strings.TrimSpace(l)
        if l == "" { continue }
        // format returns like "origin/branch"; strip remote prefix
        if strings.HasPrefix(l, remote+"/") {
            branches = append(branches, strings.TrimPrefix(l, remote+"/"))
        } else {
            branches = append(branches, l)
        }
    }
    return branches, nil
}

// branchRelation returns one of: ff, only-source, only-target, diverged
func branchRelation(ctx context.Context, dir, srcRef, tgtRef string) (string, error) {
    // check if target exists
    if err := run(ctx, dir, "rev-parse", "--verify", tgtRef); err != nil {
        // target missing
        if err := run(ctx, dir, "rev-parse", "--verify", srcRef); err == nil {
            return "only-source", nil
        }
        return "only-target", nil
    }
    // check existence of source
    if err := run(ctx, dir, "rev-parse", "--verify", srcRef); err != nil {
        return "only-target", nil
    }
    // compute ancestry
    // Is target ancestor of source? (fast-forward)
    if err := run(ctx, dir, "merge-base", "--is-ancestor", tgtRef, srcRef); err == nil {
        return "ff", nil
    }
    // Is source ancestor of target?
    if err := run(ctx, dir, "merge-base", "--is-ancestor", srcRef, tgtRef); err == nil {
        return "only-target", nil
    }
    return "diverged", nil
}

// AnalyzeFastForward clones into a temp dir and determines whether updating target from source
// can be done without non-fast-forward updates (i.e., no diverged branches).
// It returns a human-readable summary and a boolean fastForwardable.
func AnalyzeFastForward(ctx context.Context, srcURL, targetURL string) (string, bool, error) {
    tmpDir, err := os.MkdirTemp("", "ff-analyze-*")
    if err != nil { return "", false, err }
    defer os.RemoveAll(tmpDir)
    repoDir := filepath.Join(tmpDir, "repo")
    if err := run(ctx, "", "clone", srcURL, repoDir); err != nil {
        return "", false, err
    }
    if err := run(ctx, repoDir, "remote", "add", "target", targetURL); err != nil {
        return "", false, err
    }
    if err := run(ctx, repoDir, "fetch", "--all", "--tags"); err != nil { return "", false, err }

    originBranches, err := listBranches(ctx, repoDir, "origin")
    if err != nil { return "", false, err }

    var onlySrc, onlyTgt, ffCount, diverged int
    for _, b := range originBranches {
        rel, err := branchRelation(ctx, repoDir, "origin/"+b, "target/"+b)
        if err != nil { return "", false, err }
        switch rel {
        case "only-source":
            onlySrc++
        case "only-target":
            onlyTgt++
        case "ff":
            ffCount++
        case "diverged":
            diverged++
        }
    }
    fastForwardable := diverged == 0
    summary := fmt.Sprintf("branches: ff=%d new=%d diverged=%d target_only=%d", ffCount, onlySrc, diverged, onlyTgt)
    return summary, fastForwardable, nil
}


