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

try:
    import requests
except ImportError:
    requests = None

try:
    import yaml
except ImportError:
    yaml = None


DOCKER_HUB_API_URL = "https://hub.docker.com/v2/repositories/kindest/node/tags"
VERSIONS_FILE = "k8s_versions.json"
E2E_YAML_PATH = ".gitlab/test/e2e/e2e.yml"


def _check_dependencies():
    """Check if required dependencies are installed."""
    missing = []
    if requests is None:
        missing.append('requests')
    if yaml is None:
        missing.append('pyyaml')

    if missing:
        raise Exit(
            f"Missing required dependencies: {', '.join(missing)}\n" f"Install with: pip install {' '.join(missing)}",
            code=1,
        )


def _parse_version(version_str: str) -> tuple[int, int, int] | None:
    """
    Parse a Kubernetes version string like 'v1.34.0' into a tuple (1, 34, 0).
    Returns None if the version string is invalid.
    """
    match = re.match(r'^v?(\d+)\.(\d+)\.(\d+)$', version_str)
    if match:
        return tuple(map(int, match.groups()))
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
    tags = _get_docker_hub_tags()

    # Filter for valid Kubernetes version tags
    version_tags = []
    for tag in tags:
        tag_name = tag.get('name', '')
        version = _parse_version(tag_name)

        if version:
            digest = _extract_index_digest(tag)
            if digest:
                version_tags.append({'version': version, 'tag': tag_name, 'digest': digest})

    # Sort by version (major, minor, patch)
    version_tags.sort(key=lambda x: x['version'], reverse=True)

    # Return only the single latest version
    if version_tags:
        latest = version_tags[0]
        return {latest['tag']: {'digest': latest['digest'], 'tag': latest['tag']}}

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
    """Find versions that are new or have different digests."""
    new_versions = {}

    for version, data in current.items():
        if version not in previous or previous[version]['digest'] != data['digest']:
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


def _parse_existing_k8s_versions(content: str, matrix_start: int, matrix_end: int) -> list[dict[str, str]]:
    """
    Parse existing Kubernetes versions from the matrix section.
    Returns a list of dicts with 'version' and 'digest' keys.
    """
    lines = content.split('\n')
    versions = []

    # Pattern to match Kubernetes version entries
    pattern = r'kubernetesVersion=(v?\d+\.\d+(?:\.\d+)?(?:@sha256:[a-f0-9]+)?)'

    for i in range(matrix_start, matrix_end):
        line = lines[i]
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


def _find_k8s_latest_job(content: str) -> tuple:
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

    pattern = r'kubernetesVersion=(v?\d+\.\d+(?:\.\d+)?(?:@sha256:[a-f0-9]+)?)'
    match = re.search(pattern, line)

    if match:
        version_str = match.group(1)
        # Check if it has a digest
        if '@sha256:' in version_str:
            version, digest = version_str.split('@')
            return {'version': version, 'digest': digest}

    return None


def _update_e2e_yaml_file(new_versions: dict[str, dict[str, str]]) -> tuple:
    """
    Update the e2e.yml file with new Kubernetes versions.

    1. Reads the desired latest version from new_versions
    2. Reads the current latest version from new-e2e-containers-k8s-latest job
    3. If they differ, adds the new version to the matrix and updates the new-e2e-containers-k8s-latest job

    Returns (success: bool, added_versions: List[str])
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

    desired_latest_version = list(new_versions.keys())[0]
    desired_latest_digest = new_versions[desired_latest_version]['digest']
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

    # 3. If they differ, adds the new version to the matrix and updates the new-e2e-containers-k8s-latest job
    matrix_start, matrix_end, indent = _find_matrix_section(content)
    _, _, extra_params_line = _find_k8s_latest_job(content)

    if matrix_start is None:
        raise Exit("Error: Could not find matrix section in e2e.yml", code=1)

    if extra_params_line is None:
        raise Exit("Error: Could not find new-e2e-containers-k8s-latest job", code=1)

    lines = content.split('\n')
    added_versions = []

    # Parse versions only from the matrix section to avoid matching the k8s-latest job
    existing_versions = _parse_existing_k8s_versions(content, matrix_start, matrix_end)
    existing_version_strs = {v['version'] for v in existing_versions}

    if current_latest['version'] not in existing_version_strs:
        print(f"Rotating {current_latest['version']} from k8s-latest job to matrix")

        last_k8s_line = matrix_start
        for i in range(matrix_start, matrix_end):
            if 'kubernetesVersion=' in lines[i]:
                last_k8s_line = i

        # Create matrix entry for the old latest
        matrix_entry = _create_matrix_entry(
            current_latest['version'],
            current_latest['digest'],
            indent
        )

        # Insert after the last Kubernetes version
        insert_position = last_k8s_line + 1
        lines.insert(insert_position, matrix_entry)

        # Update extra_params_line index since we inserted a line before it
        if insert_position <= extra_params_line:
            extra_params_line += 1

        added_versions.append(current_latest['version'])

    print(f"Updating new-e2e-containers-k8s-latest job to {desired_latest_version}")
    old_line = lines[extra_params_line]
    new_line = re.sub(
        r'kubernetesVersion=v?\d+\.\d+\.\d+@sha256:[a-f0-9]+',
        f'kubernetesVersion={desired_latest_version}@{desired_latest_digest}',
        old_line
    )
    lines[extra_params_line] = new_line

    new_content = '\n'.join(lines)
    with open(E2E_YAML_PATH, 'w') as f:
        f.write(new_content)

    print(f"Successfully updated {E2E_YAML_PATH}")
    return True, [desired_latest_version]


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

    print("Fetching latest Kubernetes version from Docker Hub...")
    current_versions = _get_latest_k8s_versions()

    if not current_versions:
        print("Error: Could not find any Kubernetes versions")
        _set_github_output('has_new_versions', 'false')
        raise Exit("No Kubernetes versions found", code=1)

    # Show the latest version
    latest_version = list(current_versions.keys())[0]
    latest_data = current_versions[latest_version]
    print(f"Latest Kubernetes version: {latest_version}")
    print(f"  Digest: {latest_data['digest']}")

    # Load previous versions and compare
    previous_versions = _load_existing_versions(output_file)
    new_versions = _find_new_versions(current_versions, previous_versions)

    if new_versions:
        print("\nNew version found!")
        for version, data in new_versions.items():
            print(f"  {version}: {data['digest']}")

        # Save current versions for next run
        _save_versions(current_versions, output_file)

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

    This task reads Kubernetes versions from a JSON file:
    1. Compares with the current version in new-e2e-containers-k8s-latest job
    2. If they differ:
       - Moves the current version from k8s-latest job to the matrix
       - Updates k8s-latest job with the new version

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
        print(f"\nSuccessfully updated to latest version: {added_versions[0]}")
    else:
        _set_github_output('updated', 'false')
        print("\nNo updates made")
