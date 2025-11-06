# Testing the YUM Repository

This guide explains how to test the YUM/DNF repository setup.

## Prerequisites

1. A RHEL/AlmaLinux/CentOS system (or a container)
2. The repository must be published (after a release tag is pushed)

## Testing Steps

### Option 1: Test with a Container (Recommended)

```bash
# Start an AlmaLinux container
docker run -it --rm almalinux:9 bash

# Inside the container, install required tools
dnf install -y curl gnupg2 dnf-plugins-core

# Import the GPG key
curl -sSL https://joelgrimberg.github.io/repository-migrator/yum/public.key | gpg --dearmor -o /etc/pki/rpm-gpg/RPM-GPG-KEY-repository-migrator

# Add the repository
cat > /etc/yum.repos.d/repository-migrator.repo <<EOF
[repository-migrator]
name=Repository Migrator
baseurl=https://joelgrimberg.github.io/repository-migrator/yum
enabled=1
gpgcheck=1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-repository-migrator
EOF

# Test repository access
dnf makecache

# List available packages
dnf list available repository-migrator

# Install the package
dnf install -y repository-migrator

# Verify installation
repository-migrator --help
which repository-migrator
rpm -qi repository-migrator
```

### Option 2: Test on a Real System

1. **Import the GPG key:**
   ```bash
   curl -sSL https://joelgrimberg.github.io/repository-migrator/yum/public.key | sudo gpg --dearmor -o /etc/pki/rpm-gpg/RPM-GPG-KEY-repository-migrator
   ```

2. **Add the repository:**
   ```bash
   sudo tee /etc/yum.repos.d/repository-migrator.repo <<EOF
   [repository-migrator]
   name=Repository Migrator
   baseurl=https://joelgrimberg.github.io/repository-migrator/yum
   enabled=1
   gpgcheck=1
   gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-repository-migrator
   EOF
   ```

3. **Test repository access:**
   ```bash
   sudo dnf makecache
   # or on older systems
   sudo yum makecache
   ```

4. **List available packages:**
   ```bash
   dnf list available repository-migrator
   # or
   yum list available repository-migrator
   ```

5. **Install the package:**
   ```bash
   sudo dnf install repository-migrator
   # or
   sudo yum install repository-migrator
   ```

6. **Verify installation:**
   ```bash
   repository-migrator --help
   which repository-migrator
   rpm -qi repository-migrator
   ```

## Testing Repository Structure

You can verify the repository structure is correct by checking the GitHub Pages URL:

```bash
# Check repository metadata exists
curl -I https://joelgrimberg.github.io/repository-migrator/yum/repodata/repomd.xml

# Check GPG signature exists
curl -I https://joelgrimberg.github.io/repository-migrator/yum/repodata/repomd.xml.asc

# List RPM files
curl -s https://joelgrimberg.github.io/repository-migrator/yum/ | grep -o 'href="[^"]*\.rpm"' | sed 's/href="//;s/"//'
```

## Troubleshooting

### Repository not found (404)

- Wait for the GitHub Actions workflow to complete after a release tag
- Check that `docs/yum/` exists in the `main` branch
- Verify GitHub Pages is enabled for the repository

### GPG key verification fails

- Ensure the GPG key was imported correctly
- Check that `gpgcheck=1` is set in the repo file
- Verify the public key URL is accessible

### Package not found

- Run `dnf makecache` or `yum makecache` to refresh metadata
- Check that RPMs exist in the repository: `curl https://joelgrimberg.github.io/repository-migrator/yum/`
- Verify the package name matches: `repository-migrator`

### Architecture mismatch

- Ensure you're using the correct architecture (amd64 or arm64)
- Check available packages: `dnf list available repository-migrator`

## Expected Repository Structure

After a successful release, `docs/yum/` should contain:

```
docs/yum/
├── public.key                    # GPG public key
├── repodata/
│   ├── repomd.xml               # Repository metadata
│   ├── repomd.xml.asc           # GPG signature
│   ├── primary.xml.gz
│   ├── filelists.xml.gz
│   └── other.xml.gz
├── repository-migrator_<version>_linux_amd64.rpm
└── repository-migrator_<version>_linux_arm64.rpm
```

