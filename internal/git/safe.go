package git

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "os/exec"
    "strings"
)

// TagConflict describes a tag that exists on both source and target remotes
// but points to different commits. Reconciliation skips the tag rather than
// force-overwriting either side.
type TagConflict struct {
    Name      string
    SourceSHA string
    TargetSHA string
}

// Error formats a TagConflict for inclusion in a wrapped error message.
func (c TagConflict) Error() string {
    return fmt.Sprintf("tag %s: source=%s target=%s", c.Name, c.SourceSHA, c.TargetSHA)
}

// SafeRebaseAndPush clones source (non-bare, no tags), adds target remote, fetches
// branches and namespaced tags from each remote, and for each source branch:
// - fast-forward pushes when possible
// - if diverged, attempts rebase (without pushing); stops on conflicts
//
// Tags are reconciled per-tag: new source tags are pushed, identical tags are
// skipped, and tags that disagree (same name, different SHA) are returned as
// TagConflict entries without being force-pushed. If any conflicts are found,
// returns a non-nil error along with the conflicts so the caller can log them.
func SafeRebaseAndPush(ctx context.Context, srcURL, targetURL, workDir string) ([]TagConflict, error) {
    // clone non-bare, skip default tag fetch — we fetch tags into namespaced refs below
    if err := run(ctx, "", "clone", "--no-tags", srcURL, workDir); err != nil {
        return nil, err
    }
    if err := run(ctx, workDir, "remote", "add", "target", targetURL); err != nil {
        return nil, err
    }
    if err := fetchNamespacedTags(ctx, workDir, "origin"); err != nil {
        return nil, err
    }
    if err := fetchNamespacedTags(ctx, workDir, "target"); err != nil {
        return nil, err
    }

    // list branches on origin
    originBranches, err := listBranches(ctx, workDir, "origin")
    if err != nil { return nil, err }

    for _, b := range originBranches {
        srcRef := "origin/" + b
        tgtRef := "target/" + b
        relation, err := branchRelation(ctx, workDir, srcRef, tgtRef)
        if err != nil { return nil, fmt.Errorf("analyze %s: %w", b, err) }
        switch relation {
        case "only-source":
            if err := run(ctx, workDir, "push", "target", srcRef+":refs/heads/"+b); err != nil { return nil, err }
        case "ff":
            if err := run(ctx, workDir, "push", "target", srcRef+":refs/heads/"+b); err != nil { return nil, err }
        case "only-target":
            continue
        case "diverged":
            if err := run(ctx, workDir, "checkout", "-B", "_tmp_rebase_"+b, srcRef); err != nil { return nil, err }
            rebaseErr := exec.CommandContext(ctx, "git", "rebase", tgtRef)
            rebaseErr.Dir = workDir
            out, err := rebaseErr.CombinedOutput()
            if err != nil {
                _ = run(ctx, workDir, "rebase", "--abort")
                return nil, fmt.Errorf("rebase for branch %s has conflicts or failed:\n%s", b, string(out))
            }
            return nil, fmt.Errorf("branch %s diverged: a clean rebase is possible. Review locally before pushing.", b)
        }
    }

    // Reconcile tags one-by-one. New tags push; identical tags skip;
    // conflicting tags (same name, different SHA) are reported back, not clobbered.
    _, conflicts, err := reconcileTags(ctx, workDir)
    if err != nil {
        return conflicts, err
    }
    if len(conflicts) > 0 {
        names := make([]string, 0, len(conflicts))
        for _, c := range conflicts {
            names = append(names, c.Name)
        }
        return conflicts, fmt.Errorf("tag conflicts (source/target disagree on SHA): %s", strings.Join(names, ", "))
    }
    return nil, nil
}

// fetchNamespacedTags fetches branches (no tags) and tags-into-namespace from a remote.
// After this runs for remote "origin", source tags live at refs/tags/origin/<name>
// (instead of refs/tags/<name>), so the two remotes' tags cannot clobber each other.
func fetchNamespacedTags(ctx context.Context, dir, remote string) error {
    if err := run(ctx, dir, "fetch", "--no-tags", remote); err != nil {
        return err
    }
    return run(ctx, dir, "fetch", remote, "+refs/tags/*:refs/tags/"+remote+"/*")
}

// reconcileTags walks source-side tags (refs/tags/origin/*) and reconciles them
// against target-side tags (refs/tags/target/*). New tags are pushed to target;
// identical tags are no-ops; tags that disagree are recorded as conflicts and
// NOT force-pushed. Returns the names of pushed tags and the list of conflicts.
func reconcileTags(ctx context.Context, dir string) (pushed []string, conflicts []TagConflict, err error) {
    srcTags, err := listNamespacedTags(ctx, dir, "origin")
    if err != nil { return nil, nil, err }
    tgtTags, err := listNamespacedTags(ctx, dir, "target")
    if err != nil { return nil, nil, err }

    for name, srcSHA := range srcTags {
        if tgtSHA, ok := tgtTags[name]; ok {
            if tgtSHA == srcSHA {
                continue
            }
            conflicts = append(conflicts, TagConflict{Name: name, SourceSHA: srcSHA, TargetSHA: tgtSHA})
            continue
        }
        srcRef := "refs/tags/origin/" + name
        dstRef := "refs/tags/" + name
        if err := run(ctx, dir, "push", "target", srcRef+":"+dstRef); err != nil {
            return pushed, conflicts, fmt.Errorf("push tag %s: %w", name, err)
        }
        pushed = append(pushed, name)
    }
    return pushed, conflicts, nil
}

// listNamespacedTags returns a map of tag name -> object SHA from refs/tags/<remote>/*.
func listNamespacedTags(ctx context.Context, dir, remote string) (map[string]string, error) {
    cmd := exec.CommandContext(ctx, "git", "for-each-ref",
        "--format=%(refname:lstrip=3) %(objectname)",
        "refs/tags/"+remote)
    cmd.Dir = dir
    out, err := cmd.Output()
    if err != nil { return nil, err }
    tags := map[string]string{}
    for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
        line = strings.TrimSpace(line)
        if line == "" { continue }
        parts := strings.Fields(line)
        if len(parts) < 2 { continue }
        tags[parts[0]] = parts[1]
    }
    return tags, nil
}

// TagDivergence returns the output of `git log --left-right --oneline
// refs/tags/origin/<name>...refs/tags/target/<name>` truncated to maxLines.
// Lines starting with `<` are commits reachable only from the source-side tag;
// lines starting with `>` are commits reachable only from the target-side tag.
// Intended for human-readable diagnostics when a TagConflict is encountered.
func TagDivergence(ctx context.Context, dir, name string, maxLines int) (string, error) {
    cmd := exec.CommandContext(ctx, "git", "log", "--left-right", "--oneline",
        "refs/tags/origin/"+name+"...refs/tags/target/"+name)
    cmd.Dir = dir
    out, err := cmd.CombinedOutput()
    if err != nil {
        return string(out), err
    }
    lines := strings.Split(strings.TrimSpace(string(out)), "\n")
    if maxLines > 0 && len(lines) > maxLines {
        lines = append(lines[:maxLines], fmt.Sprintf("... (%d more)", len(lines)-maxLines))
    }
    return strings.Join(lines, "\n"), nil
}

func listBranches(ctx context.Context, dir string, remote string) ([]string, error) {
    cmd := exec.CommandContext(ctx, "git", "for-each-ref", fmt.Sprintf("refs/remotes/%s", remote), "--format=%(refname:short)")
    cmd.Dir = dir
    out, err := cmd.Output()
    if err != nil { return nil, err }
    lines := strings.Split(strings.TrimSpace(string(out)), "\n")
    var branches []string
    for _, l := range lines {
        l = strings.TrimSpace(l)
        if l == "" { continue }
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
    if err := run(ctx, dir, "rev-parse", "--verify", tgtRef); err != nil {
        if err := run(ctx, dir, "rev-parse", "--verify", srcRef); err == nil {
            return "only-source", nil
        }
        return "only-target", nil
    }
    if err := run(ctx, dir, "rev-parse", "--verify", srcRef); err != nil {
        return "only-target", nil
    }
    if err := run(ctx, dir, "merge-base", "--is-ancestor", tgtRef, srcRef); err == nil {
        return "ff", nil
    }
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
    if err := run(ctx, "", "clone", "--no-tags", srcURL, repoDir); err != nil {
        return "", false, err
    }
    if err := run(ctx, repoDir, "remote", "add", "target", targetURL); err != nil {
        return "", false, err
    }
    if err := fetchNamespacedTags(ctx, repoDir, "origin"); err != nil { return "", false, err }
    if err := fetchNamespacedTags(ctx, repoDir, "target"); err != nil { return "", false, err }

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

    srcTags, err := listNamespacedTags(ctx, repoDir, "origin")
    if err != nil { return "", false, err }
    tgtTags, err := listNamespacedTags(ctx, repoDir, "target")
    if err != nil { return "", false, err }
    var tagConflicts int
    for name, sha := range srcTags {
        if t, ok := tgtTags[name]; ok && t != sha {
            tagConflicts++
        }
    }

    fastForwardable := diverged == 0 && tagConflicts == 0
    summary := fmt.Sprintf("branches: ff=%d new=%d diverged=%d target_only=%d tag_conflicts=%d",
        ffCount, onlySrc, diverged, onlyTgt, tagConflicts)
    return summary, fastForwardable, nil
}
