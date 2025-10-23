#!/usr/bin/env python3
"""
Fetch the latest Kubernetes versions from Docker Hub's kindest/node repository.
This script fetches all tags, filters for valid Kubernetes versions, and outputs
the latest versions with their index digests.
"""

import json
import os
import re
import sys
from typing import Dict, List, Optional, Tuple
import requests


DOCKER_HUB_API_URL = "https://hub.docker.com/v2/repositories/kindest/node/tags"
VERSIONS_FILE = ".github/scripts/k8s_versions.json"


def parse_version(version_str: str) -> Optional[Tuple[int, int, int]]:
    """
    Parse a Kubernetes version string like 'v1.34.0' into a tuple (1, 34, 0).
    Returns None if the version string is invalid.
    """
    match = re.match(r'^v?(\d+)\.(\d+)\.(\d+)$', version_str)
    if match:
        return tuple(map(int, match.groups()))
    return None


def get_docker_hub_tags() -> List[Dict]:
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
            print(f"Error fetching tags from Docker Hub: {e}", file=sys.stderr)
            sys.exit(1)

    return all_tags


def extract_index_digest(tag_data: Dict) -> Optional[str]:
    """
    Extract the index digest (manifest list digest) from tag data.
    The index digest is the digest field at the root level of the tag data.
    """
    # The digest at the root level is the manifest list/index digest
    return tag_data.get('digest')


def get_latest_k8s_versions() -> Dict[str, Dict[str, str]]:
    """
    Fetch and parse the latest Kubernetes version from Docker Hub.
    Returns a dictionary with only the single latest version:
    {
        "v1.34.0": {
            "digest": "sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a",
            "tag": "v1.34.0"
        }
    }
    """
    tags = get_docker_hub_tags()

    # Filter for valid Kubernetes version tags
    version_tags = []
    for tag in tags:
        tag_name = tag.get('name', '')
        version = parse_version(tag_name)

        if version:
            digest = extract_index_digest(tag)
            if digest:
                version_tags.append({
                    'version': version,
                    'tag': tag_name,
                    'digest': digest
                })

    # Sort by version (major, minor, patch)
    version_tags.sort(key=lambda x: x['version'], reverse=True)

    # Return only the single latest version
    if version_tags:
        latest = version_tags[0]
        return {
            latest['tag']: {
                'digest': latest['digest'],
                'tag': latest['tag']
            }
        }

    return {}


def load_existing_versions() -> Dict[str, Dict[str, str]]:
    """Load previously stored versions from file."""
    if os.path.exists(VERSIONS_FILE):
        try:
            with open(VERSIONS_FILE, 'r') as f:
                return json.load(f)
        except (json.JSONDecodeError, IOError) as e:
            print(f"Warning: Could not load existing versions: {e}", file=sys.stderr)
    return {}


def save_versions(versions: Dict[str, Dict[str, str]]) -> None:
    """Save versions to file for future comparison."""
    os.makedirs(os.path.dirname(VERSIONS_FILE), exist_ok=True)
    with open(VERSIONS_FILE, 'w') as f:
        json.dump(versions, f, indent=2)


def find_new_versions(
    current: Dict[str, Dict[str, str]],
    previous: Dict[str, Dict[str, str]]
) -> Dict[str, Dict[str, str]]:
    """Find versions that are new or have different digests."""
    new_versions = {}

    for version, data in current.items():
        if version not in previous or previous[version]['digest'] != data['digest']:
            new_versions[version] = data

    return new_versions


def set_github_output(name: str, value: str) -> None:
    """Set a GitHub Actions output variable."""
    github_output = os.getenv('GITHUB_OUTPUT')
    if github_output:
        with open(github_output, 'a') as f:
            f.write(f"{name}={value}\n")
    else:
        print(f"::set-output name={name}::{value}")


def main():
    print("Fetching latest Kubernetes version from Docker Hub...")
    current_versions = get_latest_k8s_versions()

    if not current_versions:
        print("Error: Could not find any Kubernetes versions")
        set_github_output('has_new_versions', 'false')
        return

    # Show the latest version
    latest_version = list(current_versions.keys())[0]
    latest_data = current_versions[latest_version]
    print(f"Latest Kubernetes version: {latest_version}")
    print(f"  Digest: {latest_data['digest']}")

    # Load previous versions and compare
    previous_versions = load_existing_versions()
    new_versions = find_new_versions(current_versions, previous_versions)

    if new_versions:
        print(f"\nNew version found!")
        for version, data in new_versions.items():
            print(f"  {version}: {data['digest']}")

        # Save current versions for next run
        save_versions(current_versions)

        # Set GitHub Actions outputs
        set_github_output('has_new_versions', 'true')
        set_github_output('new_versions', json.dumps(new_versions))
    else:
        print(f"\nNo new version - {latest_version} is already tracked")
        set_github_output('has_new_versions', 'false')


if __name__ == '__main__':
    main()
