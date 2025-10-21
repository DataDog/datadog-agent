#!/usr/bin/env python3
"""
Update the .gitlab/e2e/e2e.yml file with new Kubernetes versions.
This script reads new versions from the fetch script output and updates
the matrix section in the e2e configuration.
"""

import json
import os
import re
import sys
from typing import Dict, List
import yaml


E2E_YAML_PATH = ".gitlab/e2e/e2e.yml"
VERSIONS_JSON = ".github/scripts/k8s_versions.json"


class PreserveStyleDumper(yaml.SafeDumper):
    """Custom YAML dumper to preserve style as much as possible."""
    pass


def represent_str(dumper, data):
    """Represent strings in the most appropriate style."""
    if '\n' in data:
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
    return dumper.represent_scalar('tag:yaml.org,2002:str', data)


PreserveStyleDumper.add_representer(str, represent_str)


def load_yaml_preserve_structure(file_path: str) -> str:
    """
    Load YAML file content as raw text.
    Returns the raw file content.
    """
    with open(file_path, 'r') as f:
        content = f.read()

    return content


def find_matrix_section(content: str) -> tuple:
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


def parse_existing_k8s_versions(content: str) -> List[Dict[str, str]]:
    """
    Parse existing Kubernetes versions from the matrix section.
    Returns a list of dicts with 'version' and 'digest' keys.
    """
    lines = content.split('\n')
    versions = []

    # Pattern to match Kubernetes version entries
    # Matches both: kubernetesVersion=1.33 and kubernetesVersion=v1.34.0@sha256:...
    pattern = r'kubernetesVersion=(v?\d+\.\d+(?:\.\d+)?(?:@sha256:[a-f0-9]+)?)'

    for line in lines:
        match = re.search(pattern, line)
        if match:
            version_str = match.group(1)

            # Check if it has a digest
            if '@sha256:' in version_str:
                version, digest = version_str.split('@')
                versions.append({
                    'version': version,
                    'digest': digest,
                    'line': line.strip()
                })
            else:
                # Old format without digest
                versions.append({
                    'version': version_str,
                    'digest': None,
                    'line': line.strip()
                })

    return versions


def create_matrix_entry(version: str, digest: str, indent: int) -> str:
    """
    Create a new matrix entry line for a Kubernetes version.
    """
    spaces = ' ' * indent
    return f'{spaces}- EXTRA_PARAMS: "--run TestKindSuite -c ddinfra:kubernetesVersion={version}@{digest}"'


def update_e2e_yaml(new_versions: Dict[str, Dict[str, str]]) -> tuple:
    """
    Update the e2e.yml file with new Kubernetes versions.
    Returns (success: bool, added_versions: List[str])
    """
    if not os.path.exists(E2E_YAML_PATH):
        print(f"Error: {E2E_YAML_PATH} not found", file=sys.stderr)
        return False, []

    # Load the file
    content = load_yaml_preserve_structure(E2E_YAML_PATH)

    # Find the matrix section
    matrix_start, matrix_end, indent = find_matrix_section(content)

    if matrix_start is None:
        print("Error: Could not find matrix section in e2e.yml", file=sys.stderr)
        return False, []

    # Parse existing versions
    existing_versions = parse_existing_k8s_versions(content)
    existing_version_strs = {v['version'] for v in existing_versions}

    print(f"Found {len(existing_versions)} existing Kubernetes versions in e2e.yml")

    # Determine which versions to add
    versions_to_add = []
    for version, data in new_versions.items():
        if version not in existing_version_strs:
            versions_to_add.append({
                'version': version,
                'digest': data['digest']
            })

    if not versions_to_add:
        print("No new versions to add - all versions already present")
        return False, []

    print(f"Adding {len(versions_to_add)} new version(s)")

    # Create new matrix entries
    lines = content.split('\n')
    new_entries = []

    for version_data in versions_to_add:
        entry = create_matrix_entry(
            version_data['version'],
            version_data['digest'],
            indent
        )
        new_entries.append(entry)
        print(f"  Adding: {version_data['version']}")

    # Insert new entries at the end of the matrix (before matrix_end)
    # Find the last Kubernetes version entry
    last_k8s_line = matrix_start
    for i in range(matrix_start, matrix_end):
        if 'kubernetesVersion=' in lines[i]:
            last_k8s_line = i

    # Insert after the last Kubernetes version
    insert_position = last_k8s_line + 1

    # Build the new content
    new_lines = (
        lines[:insert_position] +
        new_entries +
        lines[insert_position:]
    )

    new_content = '\n'.join(new_lines)

    # Write the updated content
    with open(E2E_YAML_PATH, 'w') as f:
        f.write(new_content)

    print(f"Successfully updated {E2E_YAML_PATH}")

    return True, [v['version'] for v in versions_to_add]


def set_github_output(name: str, value: str) -> None:
    """Set a GitHub Actions output variable."""
    github_output = os.getenv('GITHUB_OUTPUT')
    if github_output:
        with open(github_output, 'a') as f:
            f.write(f"{name}={value}\n")
    else:
        print(f"::set-output name={name}::{value}")


def main():
    # Check if there are new versions to process
    if not os.path.exists(VERSIONS_JSON):
        print("No versions file found - nothing to update")
        set_github_output('updated', 'false')
        return

    # Load new versions
    with open(VERSIONS_JSON, 'r') as f:
        all_versions = json.load(f)

    # For this script, we need to determine which versions are actually new
    # by checking what's already in e2e.yml
    print("Checking for new versions to add to e2e.yml...")

    # We'll pass all versions and let the update function determine what's new
    updated, added_versions = update_e2e_yaml(all_versions)

    if updated:
        # Format the list of new versions for the PR body
        version_list = '\n'.join(f"- `{v}`" for v in added_versions)
        set_github_output('updated', 'true')
        set_github_output('new_versions', version_list)
        print(f"\nSuccessfully added {len(added_versions)} version(s)")
    else:
        set_github_output('updated', 'false')
        print("\nNo updates made")


if __name__ == '__main__':
    main()
