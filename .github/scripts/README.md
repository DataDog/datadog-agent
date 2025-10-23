# Kubernetes Version Update Automation

This directory contains scripts that automatically update the Kubernetes versions used in e2e tests.

## Overview

The automation runs twice daily (6am and 6pm UTC) via GitHub Actions and:
1. Fetches the latest Kubernetes version from Docker Hub's `kindest/node` repository
2. Extracts the index digest for the version
3. Updates `.gitlab/e2e/e2e.yml` if a new version is available
4. Creates a pull request with the change

## Files

### `fetch_k8s_versions.py`
Fetches and parses the latest Kubernetes version from Docker Hub.

**What it does:**
- Queries the Docker Hub API for all tags of `kindest/node`
- Filters for valid Kubernetes version tags (e.g., `v1.34.0`)
- Sorts by version and finds the single latest version
- Extracts the index digest (manifest list digest) for that version
- Stores version in `k8s_versions.json` for comparison on next run
- Outputs whether a new version was found since last run

**Outputs:**
- `has_new_versions`: `true` if a new version was found
- `new_versions`: JSON object with the new version data

### `update_e2e_yaml.py`
Updates the e2e.yml file with the new Kubernetes version.

**What it does:**
- Reads the stored versions from `k8s_versions.json`
- Parses `.gitlab/e2e/e2e.yml` to find the `new-e2e-containers` matrix section
- Checks which versions are already present
- Adds new versions in the format: `kubernetesVersion=v1.34.0@sha256:...`
- Inserts new entries after the last Kubernetes version in the matrix

**Outputs:**
- `updated`: `true` if the file was updated
- `new_versions`: Markdown-formatted version for the PR description

### `k8s_versions.json`
Cache file storing the latest Kubernetes version found on Docker Hub. This file is created by `fetch_k8s_versions.py` and used to determine if a new version is available on subsequent runs.

**Note:** This file is not checked into git and is created/updated by the GitHub Actions workflow.

## GitHub Actions Workflow

The workflow is defined in `.github/workflows/update-kubernetes-versions.yml`.

**Schedule:**
- Runs twice daily at 6am and 6pm UTC
- Can also be triggered manually via workflow_dispatch

**Steps:**
1. Checkout repository
2. Setup Python and install dependencies (requests, pyyaml)
3. Run `fetch_k8s_versions.py` to get latest versions
4. Run `update_e2e_yaml.py` to update the YAML file (if new versions found)
5. Create a pull request with the changes (if updates were made)

**Permissions:**
- Uses OIDC token for authentication
- Requires `self.update-kubernetes-versions.create-pr` policy

## Manual Testing

To test the scripts locally:

```bash
# Create a virtual environment
python3 -m venv .venv
source .venv/bin/activate  # On Windows: .venv\Scripts\activate

# Install dependencies
pip install requests pyyaml

# Fetch latest versions
python .github/scripts/fetch_k8s_versions.py

# Update e2e.yml (will add versions found above)
python .github/scripts/update_e2e_yaml.py

# Check the diff
git diff .gitlab/e2e/e2e.yml

# Restore the file when done testing
git checkout .gitlab/e2e/e2e.yml
```

## How It Works

### Version Detection
The script identifies the latest Kubernetes version by:
1. Fetching all tags from `kindest/node` Docker Hub repository
2. Matching tags against the pattern `v{major}.{minor}.{patch}`
3. Sorting by version number (major, minor, patch)
4. Selecting the single highest version

### Digest Extraction
The script extracts the **index digest** (not the image digest) for the latest version:
- The index digest is the SHA256 hash of the manifest list
- This is what Kind uses to pull multi-architecture images
- It's found in the `digest` field at the root of the tag data from Docker Hub API

### YAML Update Strategy
The script:
1. Locates the `new-e2e-containers` job in `.gitlab/e2e/e2e.yml`
2. Finds the `parallel.matrix` section
3. Identifies existing Kubernetes version entries
4. Adds the new version after the last Kubernetes version entry (if not already present)
5. Maintains proper YAML indentation (6 spaces)

### PR Creation
When a new version is found:
- A new branch is created: `update-k8s-versions-{run_id}-{attempt}`
- Changes are committed with a descriptive message
- A PR is created with:
  - Title: "[automated] Add new Kubernetes version to e2e tests"
  - Labels: `team/container-integrations`, `qa/done`, `changelog/no-changelog`, `ask-review`
  - Team reviewers: `container-integrations`

## Maintenance

### Updating the Schedule
To change how often the automation runs, edit the `cron` schedule in `.github/workflows/update-kubernetes-versions.yml`:

```yaml
schedule:
  - cron: "0 6,18 * * *"  # Current: 6am and 6pm UTC
```

### Filtering Versions
To limit which Kubernetes versions can be added, modify the version detection logic in `fetch_k8s_versions.py`. For example, to only include versions >= 1.25:

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
- Review the `parse_version()` function in `fetch_k8s_versions.py`
- Check if the Docker Hub API response format has changed

**YAML formatting issues:**
- Verify the indentation detection in `update_e2e_yaml.py`
- Check that the `new-e2e-containers` job structure hasn't changed

## Dependencies

- **Python 3.11+**
- **requests**: HTTP library for API calls
- **pyyaml**: YAML parsing (minimal usage, mostly raw text manipulation)

## Related Documentation

- [Kind Node Images](https://hub.docker.com/r/kindest/node/tags)
- [Docker Hub API](https://docs.docker.com/docker-hub/api/latest/)
- [GitLab CI E2E Tests](.gitlab/e2e/e2e.yml)
