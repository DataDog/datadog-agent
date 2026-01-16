# Kubernetes Version Update Automation

This module contains tasks that automatically update the Kubernetes versions used in e2e tests.

## Overview

The automation runs twice daily (6am and 6pm UTC) via GitHub Actions and:
1. Fetches the latest Kubernetes version from Docker Hub's `kindest/node` repository
2. Extracts the index digest for the version
3. Updates the `new-e2e-containers-k8s-latest` job in `.gitlab/test/e2e/e2e.yml` if a new version is available
4. Creates a pull request with the change

**Note:** This automation maintains two separate test configurations:
- **`new-e2e-containers` job**: Uses a parallel matrix with 4 oldest K8s versions (manually maintained)
- **`new-e2e-containers-k8s-latest` job**: Tests the latest stable K8s release (automatically updated)

The matrix is never modified by this automation and should be manually rotated when needed.

## Tasks

### `k8s-versions.fetch-versions`
Fetches and parses the latest Kubernetes version from Docker Hub.

**What it does:**
- Queries the Docker Hub API for all tags of `kindest/node`
- Filters for valid Kubernetes version tags (e.g., `v1.34.0`)
- Sorts by version and finds the single latest version
- Extracts the index digest (manifest list digest) for that version
- Adds the version to `k8s_versions.json`
- Outputs whether a new version was found since last run

**Usage:**
```bash
dda inv k8s-versions.fetch-versions
```

**Outputs (GitHub Actions):**
- `has_new_versions`: `true` if a new version was found
- `new_versions`: JSON object with the new version data

### `k8s-versions.update-e2e-yaml`
Updates the e2e.yml file with the new Kubernetes version.

**What it does:**
- Reads the stored versions from `k8s_versions.json`
- Compares with current version in `new-e2e-containers-k8s-latest` job
- If different, updates `new-e2e-containers-k8s-latest` job with the new version
- **Does NOT modify** the `new-e2e-containers` matrix (manually maintained)

**Usage:**
```bash
dda inv k8s-versions.update-e2e-yaml
```

**Outputs (GitHub Actions):**
- `updated`: `true` if the file was updated
- `new_versions`: Markdown-formatted version for the PR description

## GitHub Actions Workflow

The workflow is defined in `.github/workflows/update-kubernetes-versions.yml`.

**Schedule:**
- Runs twice daily at 6am and 6pm UTC
- Can also be triggered manually via workflow_dispatch

**Steps:**
1. Checkout repository
2. Setup Python and install dda
3. Install dependencies (requests, pyyaml)
4. Run `dda inv k8s-versions.fetch-versions` to get latest versions
5. Run `dda inv k8s-versions.update-e2e-yaml` to update the YAML file (if new versions found)
6. Create a pull request with the changes (if updates were made)

**Permissions:**
- Uses OIDC token for authentication
- Requires `self.update-kubernetes-versions.create-pr` policy

## Manual Testing

To test the tasks locally:

```bash
# Install dependencies
pip install requests pyyaml

# Fetch latest versions
dda inv k8s-versions.fetch-versions

# Update e2e.yml (will add versions found above)
dda inv k8s-versions.update-e2e-yaml

# Check the diff
git diff .gitlab/test/e2e/e2e.yml

# Restore the file when done testing
git checkout .gitlab/test/e2e/e2e.yml
```

## How It Works

### Version Detection
The task identifies the latest Kubernetes version by:
1. Fetching all tags from `kindest/node` Docker Hub repository
2. Matching tags against the pattern `v{major}.{minor}.{patch}`
3. Sorting by version number (major, minor, patch)
4. Selecting the single highest version

### Digest Extraction
The task extracts the **index digest** (not the image digest) for the latest version:
- The index digest is the SHA256 hash of the manifest list
- This is what Kind uses to pull multi-architecture images
- It's found in the `digest` field at the root of the tag data from Docker Hub API

### YAML Update Strategy
The task:
1. Locates the `new-e2e-containers-k8s-latest` job in `.gitlab/test/e2e/e2e.yml`
2. Compares the desired latest version with current version in the k8s-latest job
3. If different, updates the k8s-latest job with the new version
4. **Never modifies** the `new-e2e-containers` matrix

The `new-e2e-containers` job maintains a parallel matrix with the 4 oldest K8s versions. This matrix is manually maintained and should be rotated when needed to drop older versions and include newer ones.

### PR Creation
When a new version is found:
- A new branch is created: `update-k8s-versions-{run_id}-{attempt}`
- Changes are committed with a descriptive message
- A PR is created with:
  - Title: "[automated] Update Kubernetes latest version in e2e tests"
  - Labels: `team/container-integrations`, `qa/done`, `changelog/no-changelog`, `ask-review`
  - Team reviewers: `container-integrations`
- The PR updates only the `new-e2e-containers-k8s-latest` job to test the latest stable release

## Maintenance

### Updating the Schedule
To change how often the automation runs, edit the `cron` schedule in `.github/workflows/update-kubernetes-versions.yml`:

```yaml
schedule:
  - cron: "0 6,18 * * *"  # Current: 6am and 6pm UTC
```

### Filtering Versions
To limit which Kubernetes versions can be added, modify the `_get_latest_k8s_versions()` function in `tasks/k8s_versions.py`. For example, to only include versions >= 1.25:

```python
# After sorting version_tags, filter before selecting the latest
version_tags = [
    tag for tag in version_tags
    if tag['version'][0] > 1 or (tag['version'][0] == 1 and tag['version'][1] >= 25)
]
```

### Troubleshooting

**No versions are being added:**
- Check if the Docker Hub API is accessible
- Verify the `k8s_versions.json` file is being updated
- Check the GitHub Actions workflow logs

**Wrong versions are being added:**
- Review the `_parse_version()` function in `tasks/k8s_versions.py`
- Check if the Docker Hub API response format has changed

**YAML formatting issues:**
- Check that the `new-e2e-containers-k8s-latest` job structure hasn't changed
- Verify the version replacement pattern in `_update_e2e_yaml_file()`

## Dependencies

- **Python 3.11+**
- **requests**: HTTP library for API calls
- **pyyaml**: YAML parsing (minimal usage, mostly raw text manipulation)
- **dda**: Datadog Agent development tooling

## Related Documentation

- [Kind Node Images](https://hub.docker.com/r/kindest/node/tags)
- [Docker Hub API](https://docs.docker.com/docker-hub/api/latest/)
- [GitLab CI E2E Tests](.gitlab/test/e2e/e2e.yml)
