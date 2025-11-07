package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	appcfg "repository-migrator/internal/config"
	"repository-migrator/internal/git"
	gl "repository-migrator/internal/gitlab"
	"repository-migrator/internal/logs"
	"repository-migrator/internal/util"
)

var autoCreateSubgroups = flag.Bool("auto-create-subgroups", true, "automatically create missing GitLab subgroups without prompting")
var safeRebaseMode = flag.Bool("safe-rebase", true, "attempt rebase and fast-forward only; stop on conflicts and never overwrite (default)")
var overwriteMirror = flag.Bool("overwrite", false, "overwrite target by mirroring (git push --mirror --prune)")
var configPrinted bool

func main() {
	flag.Parse()
	if args := flag.Args(); len(args) > 0 {
		// Treat first argument as repo list path for batch mode
		_ = os.Setenv("REPO_LIST_FILE", args[0])
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	runLogPath, _ := logs.BeginRunLog()
	logs.SetCurrentRunPath(runLogPath)
	cfg, err := appcfg.Load()
	if err != nil {
		return err
	}

	// Prefer env vars if present for non-interactive use
	if v := strings.TrimSpace(os.Getenv("GITLAB_BASE_URL")); v != "" {
		cfg.GitLabBaseURL = v
	}
	token := strings.TrimSpace(os.Getenv("GITLAB_TOKEN"))
	targetDefaultBranch := strings.TrimSpace(os.Getenv("TARGET_DEFAULT_BRANCH"))
	if targetDefaultBranch == "" {
		targetDefaultBranch = strings.TrimSpace(cfg.TargetDefaultBranch)
	}

	// Non-interactive mode (env or config)
	nonInteractive, _ := util.EnvBool(os.Getenv("NON_INTERACTIVE"))
	if cfg.NonInteractive {
		nonInteractive = true
	}

	// Normalize loaded config
	cfg.GitLabBaseURL = strings.TrimSpace(cfg.GitLabBaseURL)

	// If base URL present, validate and normalize; otherwise ask
	if cfg.GitLabBaseURL != "" {
		if err := validateBaseURL(cfg.GitLabBaseURL); err == nil {
			cfg.GitLabBaseURL = strings.TrimRight(cfg.GitLabBaseURL, "/")
		} else {
			// force prompt if invalid
			cfg.GitLabBaseURL = ""
		}
	}
	if cfg.GitLabBaseURL == "" {
		if nonInteractive {
			return errors.New("missing GitLab base URL in non-interactive mode (set GITLAB_BASE_URL or config.gitlab_base_url)")
		}
		val, err := util.Prompt("Enter GitLab base URL (e.g., https://gitlab.com): ")
		if err != nil {
			return err
		}
		if err := validateBaseURL(val); err != nil {
			return err
		}
		cfg.GitLabBaseURL = strings.TrimRight(val, "/")
	}
	if token == "" {
		if nonInteractive {
			return errors.New("missing GitLab token in non-interactive mode (set GITLAB_TOKEN)")
		}
		val, err := util.Prompt("Enter GitLab Personal Access Token (scope: api): ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(val) == "" {
			return errors.New("token cannot be empty")
		}
		token = strings.TrimSpace(val)
	}
	// Show configured settings (excluding token)
	if !configPrinted {
		fmt.Println("Configured settings:")
		fmt.Printf("- GitLab base URL: %s\n", cfg.GitLabBaseURL)
		if strings.TrimSpace(cfg.DefaultGroupPath) != "" {
			fmt.Printf("- Default group path: %s\n", cfg.DefaultGroupPath)
		}
		configPrinted = true
	}
	logs.AppendRunDetail(runLogPath, fmt.Sprintf("config base_url=%s default_group=%s", cfg.GitLabBaseURL, cfg.DefaultGroupPath))
	// Persist for next time
	if err := appcfg.Save(cfg); err != nil {
		return err
	}

	// Batch mode: REPO_LIST_FILE points to JSON file with repos
	if listPath := strings.TrimSpace(os.Getenv("REPO_LIST_FILE")); listPath != "" {
		return runBatch(cfg, listPath)
	}

	// Source repository URL (env SOURCE_REPO_URL or prompt)
	srcURL := strings.TrimSpace(os.Getenv("SOURCE_REPO_URL"))
	if srcURL == "" {
		if nonInteractive {
			return errors.New("missing SOURCE_REPO_URL in non-interactive mode")
		}
		val, err := util.Prompt("Enter source Git repository URL to migrate: ")
		if err != nil {
			return err
		}
		srcURL = val
	}
	if err := validateGitURL(srcURL); err != nil {
		return fmt.Errorf("invalid source URL: %w", err)
	}
	logs.AppendRunDetail(runLogPath, fmt.Sprintf("source=%s", srcURL))

	repoName := strings.TrimSpace(os.Getenv("PROJECT_NAME"))
	if repoName == "" {
		repoName = inferRepoName(srcURL)
	}
	if repoName == "" {
		if nonInteractive {
			return errors.New("missing PROJECT_NAME in non-interactive mode and could not infer from URL")
		}
		name, err := util.Prompt("Enter project name to create in GitLab: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(name) == "" {
			return errors.New("project name cannot be empty")
		}
		repoName = name
	}

	// Ask where to create project (user or group)
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := git.CheckRemote(checkCtx, srcURL); err != nil {
		cancelCheck()
		return fmt.Errorf("source repository %s not accessible: %w", srcURL, err)
	}
	cancelCheck()

	client := gl.NewClient(cfg.GitLabBaseURL, token)
	user, err := client.CurrentUser()
	if err != nil {
		return fmt.Errorf("authenticate to GitLab: %w", err)
	}
	// Collect target namespace path (from env/config) with optional subfolder; no confirmation in non-interactive
	var groupPath string
	if nonInteractive {
		nsPath := strings.TrimSpace(os.Getenv("GROUP_PATH"))
		if nsPath == "" {
			nsPath = cfg.DefaultGroupPath
		}
		subfolder := strings.TrimSpace(os.Getenv("SUBFOLDER"))
		if subfolder == "" {
			subfolder = cfg.DefaultSubfolder
		}
		candidate := strings.Trim(nsPath, "/")
		if strings.TrimSpace(subfolder) != "" {
			candidate = strings.Trim(candidate+"/"+strings.Trim(subfolder, "/"), "/")
		}
		groupPath = candidate
	} else {
		for {
			nsPath, err := util.PromptWithDefault("Namespace (group/subgroup) full path (blank for personal namespace)", cfg.DefaultGroupPath)
			if err != nil {
				return err
			}
			subfolder, err := util.PromptOptional("Subfolder within namespace (optional): ")
			if err != nil {
				return err
			}
			candidate := strings.Trim(nsPath, "/")
			if strings.TrimSpace(subfolder) != "" {
				candidate = strings.Trim(candidate+"/"+strings.Trim(subfolder, "/"), "/")
			}
			previewNamespace := candidate
			if previewNamespace == "" {
				previewNamespace = user.Username
			}
			fmt.Printf("Target namespace: %s\n", previewNamespace)
			confirm, err := util.PromptWithDefault("Proceed with this namespace? (Y/n)", "Y")
			if err != nil {
				return err
			}
			if strings.EqualFold(strings.TrimSpace(confirm), "n") || strings.EqualFold(strings.TrimSpace(confirm), "no") {
				continue
			}
			groupPath = candidate
			break
		}
	}
	// Derive project path (slug) and display name automatically from source URL
	defaultPath := strings.Trim(inferRepoName(srcURL), " ")
	projPath := sanitizeProjectPathSlug(defaultPath)
	if err := validateProjectPathSlug(projPath); err != nil {
		return fmt.Errorf("invalid derived project path '%s': %w", projPath, err)
	}
	projectName := sanitizeProjectName(strings.TrimSpace(repoName))
	if projectName == "" {
		projectName = projPath
	}

	// Persist chosen group as default for next time
	cfg.DefaultGroupPath = strings.TrimSpace(groupPath)
	if err := appcfg.Save(cfg); err != nil {
		return err
	}

	// Trial run option (env/config or prompt)
	isTrial := false
	if v, ok := util.EnvBool(os.Getenv("TRIAL_RUN")); ok {
		isTrial = v
	}
	if !isTrial && cfg.TrialRun != nil {
		isTrial = *cfg.TrialRun
	}
	if !nonInteractive && !isTrial {
		trialAns, err := util.PromptWithDefault("Trial run only? (y/N)", "N")
		if err != nil {
			return err
		}
		isTrial = strings.EqualFold(strings.TrimSpace(trialAns), "y") || strings.EqualFold(strings.TrimSpace(trialAns), "yes")
	}

	// Compute target URL without creating the project
	namespace := strings.TrimSpace(groupPath)
	if namespace == "" {
		namespace = user.Username
	}
	base := strings.TrimRight(cfg.GitLabBaseURL, "/")
	plannedURL := fmt.Sprintf("%s/%s/%s.git", base, strings.Trim(namespace, "/"), strings.Trim(projPath, "/"))

	targetDisplay := strings.TrimSuffix(plannedURL, ".git")
	sourceDisplay := strings.TrimSuffix(srcURL, ".git")
	if idxStr := strings.TrimSpace(os.Getenv("BATCH_INDEX")); idxStr != "" {
		totalStr := strings.TrimSpace(os.Getenv("BATCH_TOTAL"))
		if totalStr == "" {
			totalStr = "?"
		}
		fmt.Printf("[%s/%s] %s -> %s\n", idxStr, totalStr, sourceDisplay, targetDisplay)
	} else {
		fmt.Printf("%s -> %s\n", sourceDisplay, targetDisplay)
	}

	if isTrial {
		fmt.Println("=== TRIAL RUN - No changes will be made ===")
		fmt.Printf("Source repository: %s\n", sourceDisplay)
		fmt.Printf("Project name: %s\n", projectName)
		fmt.Printf("Project path (slug): %s\n", projPath)
		fmt.Printf("Target namespace: %s\n", namespace)
		if strings.TrimSpace(groupPath) != "" {
			fmt.Printf("Group path: %s\n", groupPath)
		}
		fmt.Printf("Target repository (planned): %s\n", targetDisplay)
		// Show mode settings
		overwrite := *overwriteMirror
		if v, ok := util.EnvBool(os.Getenv("OVERWRITE")); ok {
			overwrite = v
		}
		if cfg.Overwrite != nil {
			overwrite = *cfg.Overwrite
		}
		safe := *safeRebaseMode
		if v, ok := util.EnvBool(os.Getenv("SAFE_REBASE")); ok {
			safe = v
		}
		if cfg.SafeRebase != nil {
			safe = *cfg.SafeRebase
		}
		if overwrite {
			fmt.Println("Mode: overwrite (mirror - destructive)")
		} else if safe {
			fmt.Println("Mode: safe-rebase (fast-forward only)")
		}
		// Show auto-create subgroups setting
		acs := *autoCreateSubgroups
		if v, ok := util.EnvBool(os.Getenv("AUTO_CREATE_SUBGROUPS")); ok {
			acs = v
		}
		if cfg.AutoCreateSubgroups != nil {
			acs = *cfg.AutoCreateSubgroups
		}
		fmt.Printf("Auto-create subgroups: %v\n", acs)
		fmt.Println("===========================================")
		_ = logs.AppendMigrationLog(srcURL, plannedURL, "trial")
		logs.AppendRunDetail(runLogPath, "trial-run=true")
		return nil
	}

	// If a group/subgroup path is specified, ensure subgroup chain exists (interactive)
	if strings.TrimSpace(groupPath) != "" {
		logs.AppendRunDetail(runLogPath, fmt.Sprintf("ensure_group_chain=%s", groupPath))
		// Resolve autoCreateSubgroups from env/config/flag
		acs := *autoCreateSubgroups
		if v, ok := util.EnvBool(os.Getenv("AUTO_CREATE_SUBGROUPS")); ok {
			acs = v
		}
		if cfg.AutoCreateSubgroups != nil {
			acs = *cfg.AutoCreateSubgroups
		}
		if err := ensureGroupChain(client, groupPath, acs); err != nil {
			_ = logs.AppendMigrationLog(srcURL, plannedURL, "failed")
			logs.AppendRunDetail(runLogPath, fmt.Sprintf("ensure_group_chain_failed: %v", err))
			return err
		}
		logs.AppendRunDetail(runLogPath, "ensure_group_chain=ok")
	}

	var project gl.Project
	for {
		if strings.TrimSpace(groupPath) == "" {
			// Personal namespace
			project, err = client.CreateProjectInUserNamespace(projectName)
		} else {
			grp, gerr := client.GetGroupByFullPath(strings.TrimSpace(groupPath))
			if gerr != nil {
				return fmt.Errorf("resolve group '%s': %w", groupPath, gerr)
			}
			project, err = client.CreateProjectInNamespace(projectName, projPath, grp.ID, grp.FullPath)
		}
		if err == nil {
			logs.AppendRunDetail(runLogPath, fmt.Sprintf("project_id=%d url=%s", project.ID, project.HttpURLToRepo))
			break
		}
		// Handle path-taken scenario with re-prompt (or error in non-interactive)
		if errors.Is(err, gl.ErrProjectPathTaken) {
			if nonInteractive {
				return fmt.Errorf("project path '%s' already taken; provide a different PROJECT_NAME or slug in non-interactive mode", projPath)
			}
			fmt.Printf("The project path '%s' is already taken. Please choose a different slug.\n", projPath)
			// Re-prompt only for slug and retry
			for {
				ans, perr := util.PromptWithDefault("Project path (slug)", projPath+"-1")
				if perr != nil {
					return perr
				}
				ans = sanitizeProjectPathSlug(ans)
				if vErr := validateProjectPathSlug(ans); vErr != nil {
					fmt.Printf("Invalid project path: %v\n", vErr)
					continue
				}
				projPath = ans
				logs.AppendRunDetail(runLogPath, fmt.Sprintf("retry_slug=%s", projPath))
				break
			}
			continue
		}
		return err
	}

	// Optionally unblock default branch (allow pushes) during migration
	allowPush := false
	if v, ok := util.EnvBool(os.Getenv("ALLOW_PUSH_DEFAULT")); ok {
		allowPush = v
	}
	if !allowPush && cfg.AllowPushDefault != nil {
		allowPush = *cfg.AllowPushDefault
	}
	if !nonInteractive && !allowPush {
		unblockAns, err := util.PromptWithDefault("Temporarily allow pushes to default branch during migration? (y/N)", "N")
		if err != nil {
			return err
		}
		allowPush = strings.EqualFold(strings.TrimSpace(unblockAns), "y") || strings.EqualFold(strings.TrimSpace(unblockAns), "yes")
	}
	restoreProtection := false
	defaultBranch := project.DefaultBranch
	if allowPush && strings.TrimSpace(defaultBranch) != "" {
		if err := client.UnprotectBranch(project.ID, defaultBranch); err == nil {
			restoreProtection = true
		}
	}

	// Save chosen group path as default for next time (no-op if blank)
	cfg.DefaultGroupPath = strings.TrimSpace(groupPath)
	if err := appcfg.Save(cfg); err != nil {
		return err
	}

	// Clone and push
	workDir, err := os.MkdirTemp("", "repo-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)
	mirrorDir := filepath.Join(workDir, repoName)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Build authenticated remote URL using token; prefer oauth2 username for tokens
	targetURL, err := addTokenToHTTPURL(project.HttpURLToRepo, token)
	if err != nil {
		return err
	}

	// Show fast-forwardability summary before pushing
	ffSummary, ffOK, _ := git.AnalyzeFastForward(ctx, srcURL, targetURL)
	fmt.Printf("Fast-forward analysis: %s (fast-forwardable=%v)\n", ffSummary, ffOK)
	logs.AppendRunDetail(runLogPath, fmt.Sprintf("ff_analysis %s fast_forwardable=%v", ffSummary, ffOK))

	// Decide push strategy: derive from env/config/flags
	overwrite := *overwriteMirror
	if v, ok := util.EnvBool(os.Getenv("OVERWRITE")); ok {
		overwrite = v
	}
	if cfg.Overwrite != nil {
		overwrite = *cfg.Overwrite
	}
	safe := *safeRebaseMode
	if v, ok := util.EnvBool(os.Getenv("SAFE_REBASE")); ok {
		safe = v
	}
	if cfg.SafeRebase != nil {
		safe = *cfg.SafeRebase
	}
	if !overwrite && safe {
		logs.AppendRunDetail(runLogPath, "push_mode=safe-rebase")
		if err := git.SafeRebaseAndPush(ctx, srcURL, targetURL, mirrorDir); err != nil {
			_ = logs.AppendMigrationLog(srcURL, plannedURL, "failed")
			logs.AppendRunDetail(runLogPath, fmt.Sprintf("push_failed: %v", err))
			return err
		}
	} else {
		// legacy mirror path
		logs.AppendRunDetail(runLogPath, "push_mode=overwrite-mirror")
		if err := git.CloneMirror(ctx, srcURL, mirrorDir+".git"); err != nil {
			_ = logs.AppendMigrationLog(srcURL, plannedURL, "failed")
			logs.AppendRunDetail(runLogPath, fmt.Sprintf("clone_mirror_failed: %v", err))
			return err
		}
		if err := git.PushMirror(ctx, mirrorDir+".git", targetURL); err != nil {
			_ = logs.AppendMigrationLog(srcURL, plannedURL, "failed")
			logs.AppendRunDetail(runLogPath, fmt.Sprintf("push_mirror_failed: %v", err))
			return err
		}
	}

	fmt.Println("Migration completed successfully.")
	_ = logs.AppendMigrationLog(srcURL, plannedURL, "passed")
	logs.AppendRunDetail(runLogPath, "result=passed")
	if restoreProtection && strings.TrimSpace(defaultBranch) != "" {
		_ = client.ProtectBranch(project.ID, defaultBranch, 40, 40)
	}
	if targetDefaultBranch != "" {
		if err := client.SetDefaultBranch(project.ID, targetDefaultBranch); err != nil {
			fmt.Printf("Warning: failed to set default branch to %s: %v\n", targetDefaultBranch, err)
			logs.AppendRunDetail(runLogPath, fmt.Sprintf("set_default_branch_failed branch=%s err=%v", targetDefaultBranch, err))
		} else {
			logs.AppendRunDetail(runLogPath, fmt.Sprintf("set_default_branch=%s", targetDefaultBranch))
		}
	}
	fmt.Printf("GitLab project: %s\n", project.HttpURLToRepo)
	return nil
}

func runBatch(cfg appcfg.AppConfig, listPath string) error {
	rl, err := appcfg.LoadRepoList(listPath)
	if err != nil {
		return err
	}
	// Ensure non-interactive
	os.Setenv("NON_INTERACTIVE", "1")

	total := len(rl.Repos)
	os.Setenv("BATCH_TOTAL", strconv.Itoa(total))
	defer os.Unsetenv("BATCH_TOTAL")

	// Track results for summary
	type result struct {
		repo    string
		success bool
		err     error
	}
	results := make([]result, 0, len(rl.Repos))

	// Iterate entries
	var firstErr error
	for i, r := range rl.Repos {
		repoID := r.ProjectName
		if repoID == "" {
			repoID = fmt.Sprintf("entry-%d", i+1)
		}

		if strings.TrimSpace(r.ProjectName) == "" {
			err := fmt.Errorf("repo entry missing project_name")
			results = append(results, result{repo: repoID, success: false, err: err})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		// Derive source repo URL
		sourceURL := strings.TrimSpace(r.SourceRepoURL)
		if sourceURL == "" && strings.TrimSpace(cfg.SourceBaseURL) != "" {
			sourceName := strings.TrimSpace(r.SourceName)
			if sourceName == "" {
				sourceName = r.ProjectName
			}
			base := strings.TrimRight(cfg.SourceBaseURL, "/")
			sourceURL = base + "/" + strings.TrimLeft(sourceName, "/")
		}

		if strings.TrimSpace(sourceURL) == "" {
			err := fmt.Errorf("repo entry missing source_repo_url and config.source_base_url not set")
			results = append(results, result{repo: repoID, success: false, err: err})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		fmt.Printf("\n[%d/%d] Processing repository '%s'\n", i+1, total, r.ProjectName)

		os.Setenv("SOURCE_REPO_URL", sourceURL)
		os.Setenv("PROJECT_NAME", r.ProjectName)
		os.Setenv("BATCH_INDEX", strconv.Itoa(i+1))
		if strings.TrimSpace(r.GroupPath) != "" {
			os.Setenv("GROUP_PATH", r.GroupPath)
		} else {
			os.Unsetenv("GROUP_PATH")
		}
		if strings.TrimSpace(r.Subfolder) != "" {
			os.Setenv("SUBFOLDER", r.Subfolder)
		} else {
			os.Unsetenv("SUBFOLDER")
		}
		if r.Overwrite != nil {
			if *r.Overwrite {
				os.Setenv("OVERWRITE", "1")
			} else {
				os.Setenv("OVERWRITE", "0")
			}
		} else {
			os.Unsetenv("OVERWRITE")
		}
		if r.SafeRebase != nil {
			if *r.SafeRebase {
				os.Setenv("SAFE_REBASE", "1")
			} else {
				os.Setenv("SAFE_REBASE", "0")
			}
		} else {
			os.Unsetenv("SAFE_REBASE")
		}
		if r.TrialRun != nil {
			if *r.TrialRun {
				os.Setenv("TRIAL_RUN", "1")
			} else {
				os.Setenv("TRIAL_RUN", "0")
			}
		} else {
			os.Unsetenv("TRIAL_RUN")
		}
		if r.AllowPushDefault != nil {
			if *r.AllowPushDefault {
				os.Setenv("ALLOW_PUSH_DEFAULT", "1")
			} else {
				os.Setenv("ALLOW_PUSH_DEFAULT", "0")
			}
		} else {
			os.Unsetenv("ALLOW_PUSH_DEFAULT")
		}

		// Execute single migration
		if err := runSingle(); err != nil {
			results = append(results, result{repo: repoID, success: false, err: err})
			if firstErr == nil {
				firstErr = err
			}
		} else {
			results = append(results, result{repo: repoID, success: true})
		}
		os.Unsetenv("BATCH_INDEX")
	}

	// Print summary
	fmt.Println("\n=== Batch Migration Summary ===")
	for _, res := range results {
		if res.success {
			fmt.Printf("✅ %s\n", res.repo)
		} else {
			fmt.Printf("🔻 %s", res.repo)
			if res.err != nil {
				fmt.Printf(" - %v", res.err)
			}
			fmt.Println()
		}
	}
	fmt.Println("================================")

	return firstErr
}

// runSingle executes a single migration using current environment (batch helper).
func runSingle() error {
	prev := os.Getenv("REPO_LIST_FILE")
	_ = os.Unsetenv("REPO_LIST_FILE")
	defer func() {
		if prev != "" {
			os.Setenv("REPO_LIST_FILE", prev)
		}
	}()
	return run()
}

// ensureGroupChain ensures that each subgroup in the provided path exists under its parent.
// It prompts to create missing subgroups (non-destructive if user declines).
func ensureGroupChain(client *gl.Client, fullPath string, autoCreate bool) error {
	fullPath = strings.Trim(fullPath, "/")
	if fullPath == "" {
		return nil
	}
	parts := strings.Split(fullPath, "/")
	// Validate top-level exists
	var pathSoFar string
	for i, part := range parts {
		if i == 0 {
			pathSoFar = part
		} else {
			pathSoFar = pathSoFar + "/" + part
		}
		// Try fetch
		_, err := client.GetGroupByFullPath(pathSoFar)
		if err == nil {
			continue
		}
		// Missing - only allow creating subgroups (i>0)
		if i == 0 {
			return fmt.Errorf("group '%s' not found; cannot create top-level groups", part)
		}
		parentPath := strings.Join(parts[:i], "/")
		parent, perr := client.GetGroupByFullPath(parentPath)
		if perr != nil {
			return fmt.Errorf("resolve parent group '%s': %w", parentPath, perr)
		}
		// Maybe auto-create or prompt
		name := part
		pathSlug := sanitizeProjectPathSlug(part)
		if !autoCreate {
			fmt.Printf("Subgroup '%s' not found under '%s'. Create it now? (y/N): ", part, parent.FullPath)
			ans, aerr := util.Prompt("")
			if aerr != nil {
				return aerr
			}
			if !(strings.EqualFold(strings.TrimSpace(ans), "y") || strings.EqualFold(strings.TrimSpace(ans), "yes")) {
				return fmt.Errorf("subgroup '%s' does not exist; cannot proceed", pathSoFar)
			}
		}
		if _, cerr := client.CreateSubgroup(parent.ID, name, pathSlug); cerr != nil {
			return fmt.Errorf("create subgroup '%s': %w", pathSoFar, cerr)
		}
	}
	return nil
}

func validateBaseURL(u string) error {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("must start with http:// or https://")
	}
	_, err := url.ParseRequestURI(u)
	return err
}

func validateGitURL(u string) error {
	if strings.HasPrefix(u, "git@") { // SSH form
		return nil
	}
	_, err := url.ParseRequestURI(u)
	return err
}

func inferRepoName(u string) string {
	if u == "" {
		return ""
	}
	// Handle SSH form git@host:namespace/name.git
	if strings.HasPrefix(u, "git@") {
		parts := strings.SplitN(u, ":", 2)
		if len(parts) == 2 {
			path := parts[1]
			base := filepath.Base(strings.TrimSuffix(path, ".git"))
			return strings.TrimSuffix(base, ".git")
		}
		return ""
	}
	// HTTP(S)
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return ""
	}
	base := filepath.Base(strings.TrimSuffix(parsed.Path, ".git"))
	return strings.TrimSuffix(base, ".git")
}

func addTokenToHTTPURL(httpURL, token string) (string, error) {
	// Convert https://gitlab.com/namespace/repo.git -> https://oauth2:TOKEN@gitlab.com/namespace/repo.git
	parsed, err := url.Parse(httpURL)
	if err != nil {
		return "", err
	}
	// For PATs, GitLab supports username oauth2
	parsed.User = url.UserPassword("oauth2", token)
	return parsed.String(), nil
}

// sanitizeProjectPathSlug converts a string into a GitLab-friendly project path slug
func sanitizeProjectPathSlug(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".git")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	// Remove any character not allowed: keep a-z, 0-9, _, -, .
	re := regexp.MustCompile("[^a-z0-9_.-]+")
	s = re.ReplaceAllString(s, "-")
	// Collapse consecutive dashes
	reDash := regexp.MustCompile("-+")
	s = reDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_.")
	return s
}

func validateProjectPathSlug(s string) error {
	if s == "" {
		return errors.New("cannot be empty")
	}
	if strings.HasSuffix(s, ".git") || strings.HasSuffix(s, ".atom") {
		return errors.New("must not end with .git or .atom")
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "_") || strings.HasPrefix(s, ".") {
		return errors.New("must not start with '-', '_' or '.'")
	}
	if strings.HasSuffix(s, "-") || strings.HasSuffix(s, "_") || strings.HasSuffix(s, ".") {
		return errors.New("must not end with '-', '_' or '.'")
	}
	re := regexp.MustCompile(`^[a-z0-9_.][a-z0-9_.-]*$`)
	if !re.MatchString(s) {
		return errors.New("can only include letters, digits, '_', '-' and '.'")
	}
	return nil
}

func sanitizeProjectName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// Replace control or disallowed characters with space
	re := regexp.MustCompile(`[\p{Cc}\p{Cf}]+`)
	s = re.ReplaceAllString(s, " ")
	// Collapse whitespace
	ws := regexp.MustCompile(`\s+`)
	s = ws.ReplaceAllString(s, " ")
	return s
}
