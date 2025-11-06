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
cd /Users/joel/code/active_projects/gerrit-migrator
go mod tidy
go build -o repository-migrator
```

## Non-interactive CI usage

Set options via `~/.config/repository-migrator/config.json` and/or environment variables. If `non_interactive` is enabled (or `NON_INTERACTIVE=1`), the CLI will not prompt and will error if required values are missing.

### Config file keys (config.json)

- `gitlab_base_url` (string): GitLab base URL
- `gitlab_token` (string): Personal Access Token
- `default_group_path` (string, optional): Default group/subgroup path
- `default_subfolder` (string, optional): Subfolder appended under the group path
- `non_interactive` (bool, optional): Force non-interactive mode
- `auto_create_subgroups` (bool, optional): Auto-create missing subgroups
- `overwrite` (bool, optional): Mirror mode (destructive)
- `safe_rebase` (bool, optional): Safe fast-forward-only mode
- `trial_run` (bool, optional): Print plan and exit
- `allow_push_default_branch` (bool, optional): Temporarily unprotect default branch during migration

### Environment variables

- `GITLAB_BASE_URL`
- `GITLAB_TOKEN`
- `NON_INTERACTIVE` (1/true/yes)
- `SOURCE_REPO_URL` (required in non-interactive mode)
- `PROJECT_NAME` (required if it cannot be inferred from `SOURCE_REPO_URL`)
- `GROUP_PATH` (optional; falls back to config `default_group_path`)
- `SUBFOLDER` (optional; falls back to config `default_subfolder`)
- `TRIAL_RUN` (1/true/yes)
- `ALLOW_PUSH_DEFAULT` (1/true/yes)
- `AUTO_CREATE_SUBGROUPS` (1/true/yes)
- `OVERWRITE` (1/true/yes)
- `SAFE_REBASE` (1/true/yes)

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
- GitLab Personal Access Token (once; remembered)
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

- Subgroup auto-creation:
  - Flag: `--auto-create-subgroups`
  - Default: `true` (will create missing subgroups under the chosen group)
  - Set `--auto-create-subgroups=false` to get a confirmation prompt per missing subgroup

Environment variables (optional):

- `GITLAB_BASE_URL` – base URL (e.g., `https://gitlab.com`)
- `GITLAB_TOKEN` – Personal Access Token (scope: api)

## Where configuration is stored

- `~/.config/repository-migrator/config.json`
  - Saves: GitLab base URL, token, and default group/subgroup path
  - Token is never printed to screen or logs

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
