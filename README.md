# Repository Migrator (Git repo to GitLab)

A Go CLI to migrate any Git repository to GitLab. It remembers your GitLab domain, token, and preferred target namespace. It supports non-destructive fast‑forward migrations by default, optional overwrite mirroring, subgroup auto-creation, trial runs, and detailed run logs.

## Features

- Remembers GitLab base URL and token across runs
- Choose target namespace (group/subgroup) and optional subfolder; can auto-create missing subgroups
- Default safe mode (non-overwriting):
  - Fast‑forward pushes for branches that can be FF
  - Creates branches that exist only on source
  - Stops on diverged branches (no overwrite)
- Overwrite mode (mirror): `git push --mirror --prune` to make target match source exactly
- Trial run that prints planned source/target without changing anything
- Optional temporary unprotect of default branch during migration, then re-protect
- Detailed run logs, including GitLab API requests/responses

## Installation

### Ubuntu/Debian (via APT)

Add the APT repository and install:

```bash
# Add the repository
echo "deb [signed-by=/usr/share/keyrings/repository-migrator.gpg] https://joelgrimberg.github.io/repository-migrator/apt stable main" | sudo tee /etc/apt/sources.list.d/repository-migrator.list

# Import the GPG key
curl -sSL https://joelgrimberg.github.io/repository-migrator/apt/public.key | sudo gpg --dearmor -o /usr/share/keyrings/repository-migrator.gpg

# Update and install
sudo apt update
sudo apt install repository-migrator
```

After installation, run:

```bash
repository-migrator
```

### RHEL/AlmaLinux/CentOS (via YUM/DNF)

Add the YUM repository and install:

```bash
# Import the GPG key
curl -sSL https://joelgrimberg.github.io/repository-migrator/yum/public.key | sudo gpg --dearmor -o /etc/pki/rpm-gpg/RPM-GPG-KEY-repository-migrator

# Add the repository (for DNF/dnf-based systems)
sudo tee /etc/yum.repos.d/repository-migrator.repo <<EOF
[repository-migrator]
name=Repository Migrator
baseurl=https://joelgrimberg.github.io/repository-migrator/yum
enabled=1
gpgcheck=1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-repository-migrator
EOF

# Update and install
sudo dnf install repository-migrator
# or on older systems
sudo yum install repository-migrator
```

After installation, run:

```bash
repository-migrator
```

#### Alternative: Manual RPM install

Download the latest `.rpm` from the GitHub Release page and install:

```bash
# Example for x86_64
sudo dnf install ./repository-migrator-<version>-1.x86_64.rpm
# or on older systems
yum localinstall ./repository-migrator-<version>-1.x86_64.rpm
```

#### Alternative: Tarball (no root install required)

```bash
tar -xzf repository-migrator_<version>_linux_amd64.tar.gz
sudo mv repository-migrator /usr/local/bin/
```

### Build from source

```bash
git clone https://github.com/joelgrimberg/repository-migrator.git
cd repository-migrator
go mod tidy
go build -o repository-migrator
```

## Repository-local configuration (CI friendly)

Place these files in your repository root (next to `.gitlab-ci.yml`):

```text
./repository-migrator.config.json
./repo-list.json
.gitlab-ci.yml
```

`repository-migrator.config.json` example:

```json
{
  "gitlab_base_url": "https://gitlab.example.com",
  "source_base_url": "ssh://some_user@somedomain:123456/",
  "default_group_path": "acme/migrations",
  "default_subfolder": "legacy",
  "target_default_branch": "master",
  "non_interactive": true,
  "auto_create_subgroups": true,
  "safe_rebase": true,
  "overwrite": false,
  "trial_run": false,
  "allow_push_default_branch": false
}
```

`repo-list.json` example (used for batch migrations):

```json
{
  "repos": [
    {
      "project_name": "legacy-app",
      "group_path": "acme/migrations",
      "subfolder": "apps",
      "safe_rebase": true,
      "trial_run": true
    },
    {
      "project_name": "tools",
      "source_name": "tools.git",
      "group_path": "acme/internal",
      "overwrite": true,
      "allow_push_default_branch": true
    }
  ]
}
```

If `source_name` is omitted, the `project_name` is appended to `source_base_url` automatically.

Run the CLI manually:

```bash
# Single repository (interactive prompts, token from env)
GITLAB_TOKEN=glpat-xxxx repository-migrator

# Batch mode (non-interactive)
GITLAB_TOKEN=glpat-xxxx repository-migrator repo-list.json
```

### GitLab CI job example

```yaml
migrate-repos:
  image: registry.gitlab.com/gitlab-org/ci-cd/curl-jq:latest
  variables:
    GITLAB_TOKEN: "$MIGRATOR_TOKEN"  # store the PAT as a masked CI/CD variable
  script:
    - curl -sSL https://github.com/joelgrimberg/repository-migrator/releases/download/v1.0.16/repository-migrator_Linux_x86_64.tar.gz | tar -xz
    - chmod +x repository-migrator
    - ./repository-migrator repo-list.json
```

The job reads configuration from `repository-migrator.config.json` in the workspace and uses the `GITLAB_TOKEN` environment variable; the token is never stored on disk.

## Non-interactive CI usage

Set options via `repository-migrator.config.json` in the current working directory and/or environment variables. If `non_interactive` is enabled (or `NON_INTERACTIVE=1`), the CLI will not prompt and will error if required values are missing.

### Config file keys (repository-migrator.config.json)

- `gitlab_base_url` (string): GitLab base URL
- `source_base_url` (string, optional): Base URL for source repositories (e.g., `ssh://user@host:1234/`)
- `default_group_path` (string, optional): Default group/subgroup path
- `default_subfolder` (string, optional): Subfolder appended under the group path
- `target_default_branch` (string, optional): Set the target project's default branch after migration (defaults to `master` if omitted)
- `non_interactive` (bool, optional): Force non-interactive mode
- `auto_create_subgroups` (bool, optional): Auto-create missing subgroups
- `overwrite` (bool, optional): Mirror mode (destructive)
- `safe_rebase` (bool, optional): Safe fast-forward-only mode
- `trial_run` (bool, optional): Print plan and exit
- `allow_push_default_branch` (bool, optional): Temporarily unprotect default branch during migration

### Environment variables

- `GITLAB_BASE_URL`
- `GITLAB_TOKEN` (required for both interactive and non-interactive runs; never stored on disk)
- `NON_INTERACTIVE` (1/true/yes)
- `SOURCE_REPO_URL` (optional; derived automatically when `source_base_url` and repo `source_name` are set)
- `PROJECT_NAME` (required if it cannot be inferred from `SOURCE_REPO_URL`)
- `GROUP_PATH` (optional; falls back to config `default_group_path`)
- `SUBFOLDER` (optional; falls back to config `default_subfolder`)
- `TRIAL_RUN` (1/true/yes)
- `ALLOW_PUSH_DEFAULT` (1/true/yes)
- `AUTO_CREATE_SUBGROUPS` (1/true/yes)
- `OVERWRITE` (1/true/yes)
- `SAFE_REBASE` (1/true/yes)
- `TARGET_DEFAULT_BRANCH` (string)
- `REPO_LIST_FILE` (set automatically when you invoke `repository-migrator <path>`)

Precedence: environment variables override config; flags still apply unless overridden by env/config.

## Supported platforms

- Linux: amd64, arm64
  - Ubuntu/Debian: `.deb` via APT or tarball
  - RHEL/AlmaLinux/CentOS: `.rpm` via YUM/DNF or tarball
- macOS: amd64, arm64 (tarball)

Notes:

- Binaries are built with `CGO_ENABLED=0` for portability.
- `git` must be available on `PATH`.

## Quick start

```bash
./repository-migrator
```

You will be prompted for:

- GitLab base URL (once; remembered)
- GitLab Personal Access Token (prompted but not stored; use `GITLAB_TOKEN` env for CI)
- Source Git repository URL (SSH or HTTPS)
- Namespace (group/subgroup) and optional subfolder (remembered as default)
- Trial run? (prints plan only)
- Temporarily allow pushes to default branch? (unprotect then re-protect)

The tool then creates/reuses the project and migrates according to the selected mode.

## Modes and flags

- Default mode (safe rebase, non‑destructive):
  - Runs by default; attempts fast‑forward updates only
  - Shows a fast‑forward analysis summary before pushing

- Overwrite mode (mirror; destructive):
  - Flag: `--overwrite`
  - Mirrors all refs and prunes extra refs on target

- Security CI overlay (per-repo opt-in):
  - Flag: `--add-security-ci` (or `add_security_ci: true` in a repo-list entry)
  - After the push, commits a bundled `.gitlab-ci.yml` onto target's default branch so security scans run and feed the GitLab Security Dashboard
  - Forces overwrite-mirror semantics for this repo (overrides `--safe-rebase`)
  - See the "Security CI overlay" section below for details

- Subgroup auto-creation:
  - Flag: `--auto-create-subgroups`
  - Default: `true` (will create missing subgroups under the chosen group)
  - Set `--auto-create-subgroups=false` to get a confirmation prompt per missing subgroup

Environment variables (optional):

- `GITLAB_BASE_URL` – base URL (e.g., `https://gitlab.com`)
- `GITLAB_TOKEN` – Personal Access Token (scope: api)

## Security CI overlay

Some target repos in GitLab need a `.gitlab-ci.yml` so security scans (SAST, secret detection, dependency scanning) run and feed the GitLab Security Dashboard. The Security Dashboard only ingests scans from the project's **default branch**, so the CI file must live on `main` of target — not a side branch.

Source repos typically don't carry this file (source is the read-only truth). The migrator can therefore commit a bundled `.gitlab-ci.yml` onto target's default branch after each successful push, for repos that opt in.

### Where the template lives

Two options, in precedence order:

1. **Runtime override (recommended for operator setups)** — set `CI_TEMPLATE_FILE=/path/to/your/security-pipeline.yml` (env var) or pass `--ci-template /path/...` on the command line. The migrator reads that file at runtime and commits its contents onto target repos. Edit the file, rerun, no rebuild needed. The path takes precedence over the embedded template; a missing/unreadable path is a hard error, not a silent fallback.

2. **Embedded fallback** — if neither override is set, the migrator uses `templates/security-pipeline.yml` baked into the binary at build time. Edit that file in this repo and rebuild:

   ```bash
   go build ./...
   ```

   Pre-built binaries from the APT/YUM repositories carry whatever template shipped with that release.

Each migration's run log records which source was used:
- `template=embedded:templates/security-pipeline.yml`
- `template=/some/path/to/security-pipeline.yml`

### Opting a repo in

In a batch repo list, set `add_security_ci: true` on the entries that should receive the overlay:

```json
{
  "repos": [
    { "project_name": "needs-security",   "source_repo_url": "...", "add_security_ci": true  },
    { "project_name": "no-security-here", "source_repo_url": "..."                            }
  ]
}
```

For single-repo runs, pass `--add-security-ci` on the command line.

### What happens during a migration

1. Code is pushed from source to target (overwrite-mirror is forced when `add_security_ci: true`, so target's default branch always equals `source.main + 1 CI commit`).
2. The migrator clones target's default branch into a temp directory.
3. It writes the bundled template into `.gitlab-ci.yml`, replacing any existing copy (a warning is logged when this happens).
4. It commits with the message `chore(ci): apply security pipeline` (author/committer identity comes from `GIT_COMMITTER_NAME` / `GIT_COMMITTER_EMAIL` env vars, defaulting to `Repository Migrator <migrator@local>`).
5. It pushes that single commit to target.

Source repositories are never written to. The overlay only operates on target.

### What gets logged

In the per-run detail log (`~/.config/repository-migrator/runs/run-*.log`):

- `add_security_ci=true push_mode_forced=overwrite-mirror`
- `ci_overlay_applied=true branch=<name>`
- `ci_overlay_overwrote_source_file=true` — only when source carried its own `.gitlab-ci.yml` and the bundled template replaced it on target
- `ci_overlay_failed: <reason>` — on failure

## Where configuration is stored

- By default, `repository-migrator.config.json` in the current working directory
- Fallback: `~/.config/repository-migrator/config.json`
- Override via `REPO_MIGRATOR_CONFIG`

## Logs

- Summary log (one line per run):
- `~/.config/repository-migrator/migrations.log`
  - Includes timestamp, source, target, outcome (passed/failed/trial)

- Detailed run logs (per run):
- `~/.config/repository-migrator/runs/run-YYYYMMDDTHHMMSS±ZZZZ.log`
  - Includes config used, source URL, group creation steps, project details, fast‑forward analysis, push mode, errors, and GitLab API calls (method, URL, status, truncated body)

## Notes and caveats

Release pipeline:

- Push a tag like `v1.0.0` to trigger the GitHub Actions workflow
- GoReleaser builds tarballs and `.deb` and `.rpm` packages, and publishes a GitHub Release
- The workflow publishes APT and YUM repositories to the `gh-pages` branch under `apt/` and `yum/`

- Requires `git` available on PATH
- Default safe mode will not overwrite diverged refs or prune target-only refs
- Overwrite mode may be blocked by protected branches/rules; use the prompt to temporarily allow pushes or adjust protection settings
- Only Git refs are migrated (no issues/MRs/wiki)
