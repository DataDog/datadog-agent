"""
Tasks for managing Kubernetes version updates in e2e tests.
This module provides automation for fetching and updating Kubernetes versions
from Docker Hub's kindest/node repository.
"""

from __future__ import annotations

import json
import os
import re
import sys
from typing import TYPE_CHECKING

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.kind_node_image import get_github_rc_releases

if TYPE_CHECKING:
    import semver

try:
    import requests
except ImportError:
    requests = None

try:
    import yaml
except ImportError:
    yaml = None

try:
    import semver as _semver
except ImportError:
    _semver = None

DOCKER_HUB_API_URL = "https://hub.docker.com/v2/repositories/kindest/node/tags"
VERSIONS_FILE = "k8s_versions.json"
E2E_YAML_PATH = ".gitlab/test/e2e/e2e.yml"

# Regex pattern for Kubernetes version (release and RC supported)
# Matches: v1.35.0, v1.35.0-rc.1, etc.
K8S_VERSION_PATTERN = r'v?\d+\.\d+(?:\.\d+)?(?:-rc\.\d+)?'


def _check_dependencies():
    """Check if required dependencies are installed."""
    missing = []
    if requests is None:
        missing.append('requests')
    if yaml is None:
        missing.append('pyyaml')
    if _semver is None:
        missing.append('semver')

    if missing:
        raise Exit(
            f"Missing required dependencies: {', '.join(missing)}\n" f"Install with: pip install {' '.join(missing)}",
            code=1,
        )


def _parse_version(version_str: str) -> semver.VersionInfo | None:
    """
    Parse a Kubernetes version string into a semver VersionInfo object.

    Semver naturally handles RC versions correctly:
    - v1.35.0-rc.1 < v1.35.0-rc.2 < v1.35.0

    Examples:
        'v1.34.0' -> VersionInfo(1, 34, 0)
        'v1.35.0-rc.1' -> VersionInfo(1, 35, 0, prerelease='rc.1')

    Returns None if the version string is invalid.
    """
    # Remove leading 'v' if present
    clean_version = version_str.lstrip('v')

    try:
        return _semver.VersionInfo.parse(clean_version)
    except (ValueError, AttributeError):
        return None


def _get_docker_hub_tags() -> list[dict]:
    """
    Fetch all tags from Docker Hub for kindest/node.
    Returns a list of tag objects with name and images information.
    """
    all_tags = []
    url = DOCKER_HUB_API_URL

    while url:
        try:
            response = requests.get(url, timeout=30)
            response.raise_for_status()
            data = response.json()

            all_tags.extend(data.get('results', []))
            url = data.get('next')  # Pagination

        except requests.exceptions.RequestException as e:
            raise Exit(f"Error fetching tags from Docker Hub: {e}", code=1) from e

    return all_tags


def _extract_index_digest(tag_data: dict) -> str | None:
    """
    Extract the index digest (manifest list digest) from tag data.
    The index digest is the digest field at the root level of the tag data.
    """
    return tag_data.get('digest')


def _get_latest_k8s_versions(use_dockerhub: bool = True, use_github: bool = True) -> dict[str, dict[str, str]]:
    """
    Fetch and parse the latest Kubernetes version from Docker Hub (stable) and/or GitHub (RC).
    Returns a dictionary with only the single latest version.

    Args:
        use_dockerhub: Whether to fetch versions from Docker Hub (default: True)
        use_github: Whether to fetch RC versions from GitHub (default: True)
    """

    # Filter for valid Kubernetes version tags
    version_tags = []

    # Final release Kubernetes version tags from Docker Hub
    if use_dockerhub:
        for tag in _get_docker_hub_tags():
            tag_name = tag.get('name', '')
            version = _parse_version(tag_name)

            if version:
                digest = _extract_index_digest(tag)
                if digest:
                    version_tags.append({'version': version, 'tag': tag_name, 'digest': digest})

    # RC Kubernetes version tags from GitHub
    if use_github:
        for tag in get_github_rc_releases():
            tag_name = tag.get('tag_name', '')
            version = _parse_version(tag_name)
            if version and tag_name:
                # Hardcode 'rc' to True because get_github_rc_releases() only returns rc releases
                version_tags.append({'version': version, 'tag': tag_name, 'rc': True})

    # Sort by version (major, minor, patch)
    version_tags.sort(key=lambda x: x['version'], reverse=True)

    # Return only the single latest version
    if version_tags:
        latest = version_tags[0]

        # Parse out the necessary fields
        tag = latest.get('tag')
        digest = latest.get('digest')
        rc = latest.get('rc')

        # Build return dictionary
        # Structure: {tag_name: {'tag': tag_name, 'digest': digest?, 'rc': bool?}}
        # Final releases include 'digest', RC releases include 'rc'
        if tag:
            result = {tag: {'tag': tag}}
            if digest:
                result[tag]['digest'] = digest
            if rc:
                result[tag]['rc'] = rc
            return result

    return {}


def _load_existing_versions(versions_file: str) -> dict[str, dict[str, str]]:
    """Load previously stored versions from file."""
    if os.path.exists(versions_file):
        try:
            with open(versions_file) as f:
                return json.load(f)
        except (OSError, json.JSONDecodeError) as e:
            print(f"Warning: Could not load existing versions: {e}", file=sys.stderr)
    return {}


def _save_versions(versions: dict[str, dict[str, str]], versions_file: str) -> None:
    """Save versions to file for future comparison."""
    with open(versions_file, 'w') as f:
        json.dump(versions, f, indent=2)


def _find_new_versions(
    current: dict[str, dict[str, str]], previous: dict[str, dict[str, str]]
) -> dict[str, dict[str, str]]:
    """Find versions that are new or have different digests.

    Notes:
        - RC versions from GitHub won't have digests initially
        - Only compare digests if BOTH current and previous have them
        - If current has no digest (RC from GitHub) and version exists, don't mark as new
    """
    new_versions = {}

    for version, data in current.items():
        # Version doesn't exist in previous - it's new
        if version not in previous:
            new_versions[version] = data
            continue

        # Version exists - check if digest changed
        current_digest = data.get('digest')
        previous_digest = previous[version].get('digest')

        # Only compare digests if BOTH have them
        # This prevents RC versions (no digest from GitHub) from being marked as new
        # when they already exist in the saved file (with digest from build)
        if current_digest and previous_digest and current_digest != previous_digest:
            new_versions[version] = data

    return new_versions


def _set_github_output(name: str, value: str) -> None:
    """Set a GitHub Actions output variable."""
    github_output = os.getenv('GITHUB_OUTPUT')
    if github_output:
        with open(github_output, 'a') as f:
            f.write(f"{name}={value}\n")
    else:
        print(f"::set-output name={name}::{value}")


def _find_k8s_latest_job(content: str) -> tuple[int | None, int | None, int | None]:
    """
    Find the new-e2e-containers-k8s-latest job section in the raw content.
    Returns (job_start_line, job_end_line, extra_params_line) or (None, None, None) if not found.
    """
    lines = content.split('\n')
    in_latest_job = False
    job_start = None
    extra_params_line = None

    for i, line in enumerate(lines):
        # Find the new-e2e-containers-k8s-latest job
        if line.strip().startswith('new-e2e-containers-k8s-latest:'):
            in_latest_job = True
            job_start = i
            continue

        if in_latest_job:
            if 'EXTRA_PARAMS:' in line and 'kubernetesVersion=' in line:
                extra_params_line = i

            if line and not line[0].isspace() and line.strip().endswith(':'):
                return job_start, i, extra_params_line

    return None, None, None


def _extract_version_from_latest_job(content: str) -> dict[str, str] | None:
    """
    Extract the current Kubernetes version from the new-e2e-containers-k8s-latest job.
    Returns {'version': 'v1.34.0', 'digest': 'sha256:...'} or None if not found.
    """
    _, _, extra_params_line = _find_k8s_latest_job(content)

    if extra_params_line is None:
        return None

    lines = content.split('\n')
    line = lines[extra_params_line]

    pattern = rf'kubernetesVersion=({K8S_VERSION_PATTERN}(?:@sha256:[a-f0-9]+)?)'
    match = re.search(pattern, line)

    if match:
        version_str = match.group(1)
        # Check if it has a digest
        if '@sha256:' in version_str:
            version, digest = version_str.split('@')
            return {'version': version, 'digest': digest}

    return None


def _update_e2e_yaml_file(new_versions: dict[str, dict[str, str]]) -> tuple[bool, list[str]]:
    """
    Update the e2e.yml file with new Kubernetes versions.

    1. Reads the desired latest version from new_versions
    2. Reads the current latest version from new-e2e-containers-k8s-latest job
    3. If they differ, updates the new-e2e-containers-k8s-latest job with the new version
       (never modifies the original matrix)

    Returns (success: bool, updated_versions: List[str])
    """
    if not os.path.exists(E2E_YAML_PATH):
        raise Exit(f"Error: {E2E_YAML_PATH} not found", code=1)

    # Load the file
    with open(E2E_YAML_PATH) as f:
        content = f.read()

    # 1. Reads the desired latest version from new_versions
    if not new_versions:
        print("No new versions found")
        return False, []

    version_items = []
    for version_str, data in new_versions.items():
        parsed = _parse_version(version_str)
        if parsed:
            version_items.append((parsed, version_str, data))

    if not version_items:
        print("No valid versions found")
        return False, []

    version_items.sort(key=lambda x: x[0], reverse=True)
    desired_latest_version = version_items[0][1]
    desired_latest_digest = version_items[0][2]['digest']
    print(f"Desired latest version from new_versions: {desired_latest_version}")

    # 2. Reads the current latest version from new-e2e-containers-k8s-latest job
    current_latest = _extract_version_from_latest_job(content)

    if not current_latest:
        print("No current latest version found in new-e2e-containers-k8s-latest job")
        return False, []

    print(f"Current latest version in new-e2e-containers-k8s-latest job: {current_latest['version']}")

    if current_latest['version'] == desired_latest_version and current_latest['digest'] == desired_latest_digest:
        print("YAML is already in sync with new_versions")
        return False, []

    # 3. If they differ, update the new-e2e-containers-k8s-latest job (never modify the matrix)
    _, _, extra_params_line = _find_k8s_latest_job(content)

    if extra_params_line is None:
        raise Exit("Error: Could not find new-e2e-containers-k8s-latest job", code=1)

    lines = content.split('\n')

    print(f"Updating new-e2e-containers-k8s-latest job to {desired_latest_version}")
    old_line = lines[extra_params_line]
    new_line = re.sub(
        rf'kubernetesVersion={K8S_VERSION_PATTERN}@sha256:[a-f0-9]+',
        f'kubernetesVersion={desired_latest_version}@{desired_latest_digest}',
        old_line,
    )
    lines[extra_params_line] = new_line

    new_content = '\n'.join(lines)
    with open(E2E_YAML_PATH, 'w') as f:
        f.write(new_content)

    print(f"Successfully updated {E2E_YAML_PATH}")
    return True, [desired_latest_version]


@task
def fetch_versions(_, output_file=VERSIONS_FILE, disable_dockerhub=False, disable_github=False):
    """
    Fetch the latest Kubernetes version from Docker Hub (stable) and/or GitHub (RC).

    This task fetches the latest Kubernetes version from the kindest/node
    Docker Hub repository and/or GitHub RC releases.

    Args:
        disable_dockerhub: Disable fetching versions from Docker Hub (default: False)
        disable_github: Disable fetching RC versions from GitHub (default: False)

    Outputs (GitHub Actions):
        has_new_versions: 'true' if a new stable version was found
        has_new_rc_versions: 'true' if a new RC version was found
        new_versions: JSON string with the new version data
    """
    _check_dependencies()

    # Convert CLI arguments (--disable flags) to positive booleans
    enable_dockerhub = not disable_dockerhub
    enable_github = not disable_github

    if not enable_dockerhub and not enable_github:
        print("Error: At least one source must be enabled")
        raise Exit("No sources enabled", code=1)

    sources = []
    if enable_dockerhub:
        sources.append("Docker Hub")
    if enable_github:
        sources.append("GitHub")

    print(f"Fetching latest Kubernetes version from {' and '.join(sources)}...")
    current_versions = _get_latest_k8s_versions(use_dockerhub=enable_dockerhub, use_github=enable_github)

    if not current_versions:
        print("Error: Could not find any Kubernetes versions")
        _set_github_output('has_new_versions', 'false')
        raise Exit("No Kubernetes versions found", code=1)

    # Show the latest version
    latest_version = list(current_versions.keys())[0]
    latest_data = current_versions[latest_version]

    print(f"Latest Kubernetes version: {latest_version}")
    print(f"  Digest: {latest_data.get('digest', 'Digest unknown')}")

    # Load previous versions and compare
    previous_versions = _load_existing_versions(output_file)
    new_versions = _find_new_versions(current_versions, previous_versions)

    if new_versions:
        print("\nNew version(s) found!")
        for version, data in new_versions.items():
            print(f"  {version}: {data.get('digest', 'Digest unknown')}")

        # Check if any new versions are RCs
        has_rc = any(data.get('rc', False) for data in new_versions.values())

        # Set GitHub Actions outputs
        _set_github_output('has_new_versions', 'true')
        _set_github_output('has_new_rc_versions', 'true' if has_rc else 'false')
        _set_github_output('new_versions', json.dumps(new_versions))
    else:
        print(f"\nNo new version - {latest_version} is already tracked")
        _set_github_output('has_new_versions', 'false')
        _set_github_output('has_new_rc_versions', 'false')


@task
def update_e2e_yaml(_, versions_file=VERSIONS_FILE):
    """
    Update the e2e.yml file with new Kubernetes versions.

    This task reads Kubernetes versions from a JSON file:
    1. Compares with the current version in new-e2e-containers-k8s-latest job
    2. If they differ, updates the new-e2e-containers-k8s-latest job with the new version

    Args:
        versions_file: Path to the JSON file containing versions (default: k8s_versions.json)

    Outputs (GitHub Actions):
        updated: 'true' if the file was updated
        new_versions: Markdown-formatted list of updated versions
    """
    _check_dependencies()

    # Check if there are new versions to process
    if not os.path.exists(versions_file):
        print("No versions file found - nothing to update")
        _set_github_output('updated', 'false')
        return

    # Load new versions
    with open(versions_file) as f:
        all_versions = json.load(f)

    print("Checking for new versions to add to e2e.yml...")

    # Update the YAML file
    updated, added_versions = _update_e2e_yaml_file(all_versions)

    if updated:
        # Format the list of new versions for the PR body
        version_list = '\n'.join(f"- `{v}`" for v in added_versions)
        _set_github_output('updated', 'true')
        _set_github_output('new_versions', version_list)
        print(f"\nSuccessfully updated to latest version: {added_versions[0]}")
    else:
        _set_github_output('updated', 'false')
        print("\nNo updates made")


@task
def save_versions(_, versions, versions_file=VERSIONS_FILE):
    """
    Save multiple Kubernetes versions to the versions file.

    This task merges the provided versions with existing versions in the file,
    preserving existing entries and adding new ones.

    Args:
        versions: JSON string or dict mapping version tags to version data
                  (e.g., '{"v1.35.0": {"tag": "v1.35.0", "digest": "sha256:..."}}')
        versions_file: Path to the JSON file to store versions (default: k8s_versions.json)
    """

    # Parse if it's a JSON string
    if isinstance(versions, str):
        try:
            versions = json.loads(versions)
        except json.JSONDecodeError as e:
            raise Exit(f"Invalid JSON in versions argument: {e}", code=1) from e

    # Load existing versions
    existing_versions = _load_existing_versions(versions_file)

    # Safely append the passed in dictionary items to the version list
    for outer_tag, version in versions.items():
        inner_tag = version.get('tag')
        digest = version.get('digest')
        if not inner_tag or not digest:
            print(f"Version {outer_tag} is missing required field tag or digest, skipping...")
            continue

        existing_versions[outer_tag] = {'tag': inner_tag, 'digest': digest}

    # Save to file
    _save_versions(existing_versions, versions_file)
