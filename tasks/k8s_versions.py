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
from enum import Enum
from typing import TYPE_CHECKING

from invoke.exceptions import Exit
from invoke.tasks import task

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

GITHUB_URL_BASE = "https://api.github.com"
VERSIONS_FILE = "k8s_versions.json"
E2E_YAML_PATH = ".gitlab/test/e2e/e2e.yml"
KIND_VERSIONS_JSON_PATH = "test/e2e-framework/components/kubernetes/kind_versions.json"

# Regex pattern for Kubernetes version (release and RC supported)
# Matches: v1.35.0, v1.35.0-rc.1, etc.
K8S_VERSION_PATTERN = r'v?\d+\.\d+(?:\.\d+)?(?:-rc\.\d+)?'

class ReleaseType(Enum):
    STABLE = 1
    RC = 2

# KubernetesRelease represents a Kubernetes version fetched from Github
# Given that it is a Github release, it won't contain an image digest or kind version.
class KubernetesRelease:
    def __init__(self, tag: str):
        self.tag: str = tag
        self.semver: semver.VersionInfo = _parse_version(tag)

        # start with stable
        self.release_type: ReleaseType = ReleaseType.STABLE
        if not self.semver:
            raise ValueError(f"Version could not be parsed from {self.semver}")

        # override to RC if applicable
        if self.semver.prerelease and self.semver.prerelease.startswith("rc"):
            self.release_type = ReleaseType.RC

    def as_dict(self):
        return { 'tag': self.tag, 'rc': self.release_type == ReleaseType.RC }

# KindKubernetesImage represents an existing Kubernetes version. This means
# a version that already has a corresponding Kind image built, so it will contain
# a digest and possibly a kind_version.
class KindKubernetesImage(KubernetesRelease):
    def __init__(self, tag: str, digest: str, kind_version: str = ""):
        super().__init__(tag)

        self.digest: str = digest
        if not self.digest:
            raise ValueError(f"Digest is a required field")

        self.kind_version: str = kind_version

    @classmethod
    def from_dict(cls, data) -> KindKubernetesImage:
        tag = data.get('tag')
        digest = data.get('digest')
        kind_version = data.get('kind_version')
        return cls(tag, digest, kind_version)

    def as_dict(self):
        tmp = {'tag': self.tag, 'digest': self.digest, 'rc': self.release_type == ReleaseType.RC,}
        if self.kind_version:
            tmp['kind_version'] = self.kind_version
        return tmp


class KubernetesVersions[T: (KubernetesRelease, KindKubernetesImage)]:
    def __init__(self):
        self.versions:list[T] = []

    @classmethod
    def from_dict(cls, data, *, type_: type[T])-> KubernetesVersions[T]:
        versions = cls()
        for _, version in data.items():
            kv = type_.from_dict(version)
            versions.add(kv)
        return versions

    def as_dict(self):
        tmp = {}
        for r in self.versions:
            tmp[r.tag] = r.as_dict()
        return tmp

    def __bool__(self):
        return len(self.versions) > 0

    def __iter__(self):
        return iter(self.versions)

    def add(self, version: T):
        self.versions.append(version)

    def latest(self, release_type:ReleaseType) -> T | None:
        filtered = [r for r in self.versions if r.release_type == release_type]
        if filtered:
            return max(filtered, key=lambda x: x.semver)
        return None

    def contains(self, tag: str) -> bool:
        return any(x.tag == tag for x in self.versions)


def fetch_github_releases(version_pattern: str = K8S_VERSION_PATTERN) -> KubernetesVersions[KubernetesRelease]:
    """Get releases from Kubernetes GitHub repository."""
    discovered = KubernetesVersions[KubernetesRelease]()

    # Get the last 100 releases
    # TODO(TBD): Should we fetch all releases or is the last 25 enough?
    url = f"{GITHUB_URL_BASE}/repos/kubernetes/kubernetes/releases?per_page=25&page=1"

    try:
        response = requests.get(url, timeout=30)
        response.raise_for_status()

        for release in response.json():
            tag_name = release.get('tag_name', '')
            if re.fullmatch(version_pattern, tag_name):
                discovered.add(KubernetesRelease(tag_name))

    except requests.exceptions.RequestException as e:
        raise Exit(f"Error fetching releases from Github: {e}", code=1) from e

    return discovered


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


def _get_latest_kind_release() -> str | None:
    """
    Fetch the latest released kind version from GitHub.
    Returns the tag name (e.g. "v0.31.0") or None on failure.
    """
    try:
        resp = requests.get(
            f"{GITHUB_URL_BASE}/repos/kubernetes-sigs/kind/releases/latest",
            headers={"Accept": "application/vnd.github+json"},
            timeout=30,
        )
        resp.raise_for_status()
        return resp.json().get("tag_name")
    except Exception as e:
        print(f"Warning: could not fetch latest kind release: {e}", file=sys.stderr)
        return None


def _get_latest_releases() -> KubernetesVersions[KubernetesRelease]:
    """
    Fetch and parse the latest Kubernetes versions from GitHub releases.
    Returns a dictionary with the latest stable and RC versions.
    """

    all_versions = fetch_github_releases()
    latest_versions = KubernetesVersions[KubernetesRelease]()

    # Append the latest stable
    latest_stable = all_versions.latest(ReleaseType.STABLE)
    if latest_stable:
        latest_versions.add(latest_stable)

    # Get the latest RC and append it if:
    # - No latest_stable was found
    # - latest_rc > latest stable
    latest_rc = all_versions.latest(ReleaseType.RC)
    if latest_rc and (latest_stable is None or latest_rc.semver > latest_stable.semver):
        latest_versions.add(latest_rc)

    return latest_versions


def _load_existing_versions(versions_file: str) -> KubernetesVersions[KindKubernetesImage]:
    """Load previously stored versions from file."""
    if os.path.exists(versions_file):
        try:
            with open(versions_file) as f:
                return KubernetesVersions.from_dict(json.load(f), type_=KindKubernetesImage)

        except (OSError, json.JSONDecodeError) as e:
            print(f"Warning: Could not load existing versions: {e}", file=sys.stderr)
    return KubernetesVersions()


def _save_versions(versions: KubernetesVersions[KindKubernetesImage], versions_file: str) -> None:
    """Save versions to file for future comparison."""
    with open(versions_file, 'w') as f:
        json.dump(versions.as_dict(), f, indent=2)


def _find_new_versions(
        latest: KubernetesVersions[KubernetesRelease], previous: KubernetesVersions[KindKubernetesImage]
) -> KubernetesVersions[KubernetesRelease]:
    new_versions = KubernetesVersions[KubernetesRelease]()

    for version in latest:
        # Version doesn't exist in previous - it's new
        if not previous.contains(version.tag):
            new_versions.add(version)

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


def _extract_version_from_latest_job(content: str) -> KindKubernetesImage | None:
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
            return KindKubernetesImage(version, digest)

    return None


def _update_e2e_yaml_file(new_versions: KubernetesVersions[KindKubernetesImage]) -> tuple[bool, list[str]]:
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

    desired = new_versions.latest(ReleaseType.STABLE)
    print(f"Desired latest version from new_versions: {desired.tag}")

    # 2. Reads the current latest version from new-e2e-containers-k8s-latest job
    current_latest = _extract_version_from_latest_job(content)

    if not current_latest:
        print("No current latest version found in new-e2e-containers-k8s-latest job")
        return False, []

    print(f"Current latest version in new-e2e-containers-k8s-latest job: {current_latest.tag}")

    if current_latest.tag == desired.tag and current_latest.digest == desired.digest:
        print("YAML is already in sync with new_versions")
        return False, []

    # 3. If they differ, update the new-e2e-containers-k8s-latest job (never modify the matrix)
    _, _, extra_params_line = _find_k8s_latest_job(content)

    if extra_params_line is None:
        raise Exit("Error: Could not find new-e2e-containers-k8s-latest job", code=1)

    lines = content.split('\n')

    print(f"Updating new-e2e-containers-k8s-latest job to {desired.tag}")
    old_line = lines[extra_params_line]
    new_line = re.sub(
        rf'kubernetesVersion={K8S_VERSION_PATTERN}@sha256:[a-f0-9]+',
        f'kubernetesVersion={desired.tag}@{desired.digest}',
        old_line,
    )
    lines[extra_params_line] = new_line

    new_content = '\n'.join(lines)
    with open(E2E_YAML_PATH, 'w') as f:
        f.write(new_content)

    print(f"Successfully updated {E2E_YAML_PATH}")
    return True, [desired.tag]


@task
def fetch_versions(_, output_file=VERSIONS_FILE):
    """
    This task fetches the latest Kubernetes version from GitHub releases.

    Outputs (GitHub Actions):
        has_new_versions: 'true' if a new stable or RC version was found
        new_versions: JSON string with the new version data
    """
    _check_dependencies()

    print(f"Fetching latest Kubernetes version...")
    latest = _get_latest_releases()
    if not latest.versions:
        print("Error: Could not find any Kubernetes versions")
        _set_github_output('has_new_versions', 'false')
        raise Exit("No Kubernetes versions found", code=1)

    # Load previous versions and compare
    existing_versions = _load_existing_versions(output_file)
    new_versions = _find_new_versions(latest, existing_versions)

    if new_versions:
        print("\nNew version(s) found!")
        for version in new_versions:
            print(f"  {version.tag}")

        # Set GitHub Actions outputs
        _set_github_output('has_new_versions', 'true')
        _set_github_output('new_versions', json.dumps(new_versions.as_dict()))
    else:
        print(f"\nNo new version(s) found")
        _set_github_output('has_new_versions', 'false')


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
        all_versions = KubernetesVersions.from_dict(json.load(f), type_=KindKubernetesImage)

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
def update_kind_versions_file(_, versions_file=VERSIONS_FILE):
    """
    Update kind_versions.json with new Kubernetes versions.

    Reads versions_file (k8s_versions.json) to discover which Kubernetes minor versions
    exist, then upserts into kind_versions.json keyed by minor version ("1.35", etc.).
    The kind_version to use is taken from the existing kind_versions.json entry for that
    minor version; if no entry exists yet, the latest kind release is fetched from GitHub.
    The versions_file itself is never modified.

    Args:
        versions_file: Path to the JSON file containing versions (default: VERSIONS_FILE)
    """
    _check_dependencies()

    if not os.path.exists(versions_file):
        print("No versions file found - nothing to update")
        return

    with open(versions_file) as f:
        existing_k8s_versions = KubernetesVersions.from_dict(json.load(f), type_=KindKubernetesImage)

    # Load existing kind_versions.json
    kind_versions = {}
    if os.path.exists(KIND_VERSIONS_JSON_PATH):
        with open(KIND_VERSIONS_JSON_PATH) as f:
            kind_versions = json.load(f)

    # We fetch the latest kind release later if needed
    latest_kind_release = None
    updated = False

    for version in existing_k8s_versions:# Derive minor version key ("1.35" from "v1.35.1")
        minor_key = f"{version.semver.major}.{version.semver.minor}"
        # Resolve kind_version: prefer what's already in kind_versions.json, then
        # the version in k8s_versions.json, then fallback to the latest kind release from GitHub.
        existing = kind_versions.get(minor_key, {})
        kind_version = existing.get('kind_version') or version.get('kind_version')
        if not kind_version:
            if latest_kind_release is None:
                latest_kind_release = _get_latest_kind_release()
            kind_version = latest_kind_release
            if not kind_version:
                raise Exit(f"Could not determine kind version for {version.tag}", code=1)
            print(f"Using latest kind release {kind_version} for {minor_key}")

        node_image_version = f"{version.tag}@{version.digest}"

        if existing.get('kind_version') != kind_version or existing.get('node_image_version') != node_image_version:
            kind_versions[minor_key] = {
                'kind_version': kind_version,
                'node_image_version': node_image_version,
            }
            print(f"Updated {minor_key}: kind_version={kind_version}, node_image_version={node_image_version}")
            updated = True

    if updated:
        # Sort descending by minor version for stable diffs
        def _version_key(k):
            parts = k.split('.')
            return [int(p) for p in parts]

        sorted_versions = dict(sorted(kind_versions.items(), key=lambda x: _version_key(x[0]), reverse=True))
        with open(KIND_VERSIONS_JSON_PATH, 'w') as f:
            json.dump(sorted_versions, f, indent=2)
            f.write('\n')
        print(f"\nSuccessfully updated {KIND_VERSIONS_JSON_PATH}")
    else:
        print("\nNo updates needed for kind_versions.json")


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
            versions = KubernetesVersions.from_dict(json.loads(versions), type_=KindKubernetesImage)
        except json.JSONDecodeError as e:
            raise Exit(f"Invalid JSON in versions argument: {e}", code=1) from e

    # Load existing versions
    existing_versions = _load_existing_versions(versions_file)

    for new in versions:
        existing_versions.add(new)

    # Save to file
    _save_versions(existing_versions, versions_file)
