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

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.kind_node_image import get_github_rc_releases

try:
    import requests
except ImportError:
    requests = None

try:
    import yaml
except ImportError:
    yaml = None

try:
    import semver
except ImportError:
    semver = None

DOCKER_HUB_API_URL = "https://hub.docker.com/v2/repositories/kindest/node/tags"
VERSIONS_FILE = "k8s_versions.json"
E2E_YAML_PATH = ".gitlab/e2e/e2e.yml"


def _check_dependencies():
    """Check if required dependencies are installed."""
    missing = []
    if requests is None:
        missing.append('requests')
    if yaml is None:
        missing.append('pyyaml')
    if semver is None:
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
        return semver.VersionInfo.parse(clean_version)
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


def _get_latest_k8s_versions() -> dict[str, dict[str, str]]:
    """
    Fetch and parse the latest Kubernetes version from Docker Hub.
    Returns a dictionary with only the single latest version.
    """

    # Filter for valid Kubernetes version tags
    version_tags = []

    # Final release Kubernetes version tags
    for tag in _get_docker_hub_tags():
        tag_name = tag.get('name', '')
        version = _parse_version(tag_name)

        if version:
            digest = _extract_index_digest(tag)
            if digest:
                version_tags.append({'version': version, 'tag': tag_name, 'digest': digest})

    # RC Kubernetes version tags
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


def _find_matrix_section(content: str) -> tuple:
    """
    Find the new-e2e-containers job matrix section in the raw content.
    Returns (start_line, end_line, indent_level) or None if not found.
    """
    lines = content.split('\n')
    in_containers_job = False
    in_parallel = False
    in_matrix = False
    matrix_start = None
    indent = None

    for i, line in enumerate(lines):
        # Find the new-e2e-containers job
        if line.strip().startswith('new-e2e-containers:'):
            in_containers_job = True
            continue

        if in_containers_job:
            # Find the end of the matrix first (if we're in one)
            if in_matrix and matrix_start and indent:
                # Empty lines are ok
                if not line.strip():
                    continue
                # If the line has content and is less indented than matrix entries
                if line.strip() and not line.startswith(' ' * indent):
                    # We've left the matrix section
                    return matrix_start, i, indent

            # Check if we've left the job (another job starts at same level)
            if line and not line[0].isspace() and line.strip().endswith(':'):
                # If we were in a matrix, return what we found
                if in_matrix and matrix_start and indent:
                    return matrix_start, i, indent
                break

            # Find the parallel section
            if 'parallel:' in line:
                in_parallel = True
                continue

            # Find the matrix section within parallel
            if in_parallel and 'matrix:' in line:
                in_matrix = True
                # Look for the first EXTRA_PARAMS line to find the indent and start
                for j in range(i + 1, len(lines)):
                    if lines[j].strip().startswith('- EXTRA_PARAMS:'):
                        indent = len(lines[j]) - len(lines[j].lstrip())
                        matrix_start = j
                        break
                    # Also break if we hit a line that's not a comment/empty
                    if lines[j].strip() and not lines[j].strip().startswith('#'):
                        # If it doesn't start with -, we've gone too far
                        if not lines[j].strip().startswith('-'):
                            break
                continue

    return None, None, None


def _parse_existing_k8s_versions(content: str) -> list[dict[str, str]]:
    """
    Parse existing Kubernetes versions from the matrix section.
    Returns a list of dicts with 'version' and 'digest' keys.
    """
    lines = content.split('\n')
    versions = []

    # Pattern to match Kubernetes version entries
    pattern = r'kubernetesVersion=(v?\d+\.\d+(?:\.\d+)?(?:@sha256:[a-f0-9]+)?)'

    for line in lines:
        match = re.search(pattern, line)
        if match:
            version_str = match.group(1)

            # Check if it has a digest
            if '@sha256:' in version_str:
                version, digest = version_str.split('@')
                versions.append({'version': version, 'digest': digest, 'line': line.strip()})
            else:
                # Old format without digest
                versions.append({'version': version_str, 'digest': None, 'line': line.strip()})

    return versions


def _create_matrix_entry(version: str, digest: str, indent: int) -> str:
    """Create a new matrix entry line for a Kubernetes version."""
    spaces = ' ' * indent
    return f'{spaces}- EXTRA_PARAMS: "--run TestKindSuite -c ddinfra:kubernetesVersion={version}@{digest}"'


def _update_e2e_yaml_file(new_versions: dict[str, dict[str, str]]) -> tuple:
    """
    Update the e2e.yml file with new Kubernetes versions.
    Returns (success: bool, added_versions: List[str])
    """
    if not os.path.exists(E2E_YAML_PATH):
        raise Exit(f"Error: {E2E_YAML_PATH} not found", code=1)

    # Load the file
    with open(E2E_YAML_PATH) as f:
        content = f.read()

    # Find the matrix section
    matrix_start, matrix_end, indent = _find_matrix_section(content)

    if matrix_start is None:
        raise Exit("Error: Could not find matrix section in e2e.yml", code=1)

    # Parse existing versions
    existing_versions = _parse_existing_k8s_versions(content)
    existing_version_strs = {v['version'] for v in existing_versions}

    print(f"Found {len(existing_versions)} existing Kubernetes versions in e2e.yml")

    # Determine which versions to add
    versions_to_add = []
    for version, data in new_versions.items():
        if version not in existing_version_strs:
            versions_to_add.append({'version': version, 'digest': data['digest']})

    if not versions_to_add:
        print("No new versions to add - all versions already present")
        return False, []

    print(f"Adding {len(versions_to_add)} new version(s)")

    # Create new matrix entries
    lines = content.split('\n')
    new_entries = []

    for version_data in versions_to_add:
        entry = _create_matrix_entry(version_data['version'], version_data['digest'], indent)
        new_entries.append(entry)
        print(f"  Adding: {version_data['version']}")

    # Find the last Kubernetes version entry
    last_k8s_line = matrix_start
    for i in range(matrix_start, matrix_end):
        if 'kubernetesVersion=' in lines[i]:
            last_k8s_line = i

    # Insert after the last Kubernetes version
    insert_position = last_k8s_line + 1

    # Build the new content
    new_lines = lines[:insert_position] + new_entries + lines[insert_position:]
    new_content = '\n'.join(new_lines)

    # Write the updated content
    with open(E2E_YAML_PATH, 'w') as f:
        f.write(new_content)

    print(f"Successfully updated {E2E_YAML_PATH}")

    return True, [v['version'] for v in versions_to_add]


@task
def fetch_versions(_, output_file=VERSIONS_FILE):
    """
    Fetch the latest Kubernetes version from Docker Hub.

    This task fetches the latest Kubernetes version from the kindest/node
    Docker Hub repository and stores it in a JSON file for comparison.

    Args:
        output_file: Path to the JSON file to store versions (default: k8s_versions.json)

    Outputs (GitHub Actions):
        has_new_versions: 'true' if a new version was found
        new_versions: JSON string with the new version data
    """
    _check_dependencies()

    print("Fetching latest Kubernetes version from Docker Hub and GitHub...")
    current_versions = _get_latest_k8s_versions()

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

        # Set GitHub Actions outputs
        _set_github_output('has_new_versions', 'true')
        _set_github_output('new_versions', json.dumps(new_versions))
    else:
        print(f"\nNo new version - {latest_version} is already tracked")
        _set_github_output('has_new_versions', 'false')


@task
def update_e2e_yaml(_, versions_file=VERSIONS_FILE):
    """
    Update the e2e.yml file with new Kubernetes versions.

    This task reads Kubernetes versions from a JSON file and updates the
    .gitlab/e2e/e2e.yml file with any new versions that aren't already present.

    Args:
        versions_file: Path to the JSON file containing versions (default: k8s_versions.json)

    Outputs (GitHub Actions):
        updated: 'true' if the file was updated
        new_versions: Markdown-formatted list of added versions
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
        print(f"\nSuccessfully added {len(added_versions)} version(s)")
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
