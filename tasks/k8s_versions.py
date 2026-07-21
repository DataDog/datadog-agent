"""
Tasks for managing Kubernetes version updates in e2e tests.
This module fetches the latest Kubernetes releases from GitHub, tracks known
versions in k8s_versions.json, and updates the kubernetesVersion pinned in
the Kubernetes latest-style jobs of .gitlab/test/e2e/e2e.yml.
"""

from __future__ import annotations

import json
import os
import re
import sys
from enum import Enum
from typing import TYPE_CHECKING, NamedTuple

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

K8S_LATEST_JOB = "new-e2e-containers-k8s-latest"
K8S_RC_LATEST_JOB = "new-e2e-containers-k8s-rc-latest"

# Regex pattern for Kubernetes version (release and RC supported)
# Matches: v1.35.0, v1.35.0-rc.1, etc.
K8S_VERSION_PATTERN = r'v?\d+\.\d+(?:\.\d+)?(?:-rc\.\d+)?'


# ReleaseType is an enum that describes the Kubernetes Version release type
# We care about stable (release) versions and RC (prerelease) versions.
class ReleaseType(Enum):
    STABLE = 1
    RC = 2


# KubernetesRelease represents a Kubernetes version fetched from GitHub.
# This class does NOT contain image digest because it does not refer to
# a built image in a registry - it refers to a GitHub release.
class KubernetesRelease:
    def __init__(self, tag: str):
        self.tag: str = tag
        self.semver: semver.VersionInfo = _parse_version(tag)

        # start with stable
        self.release_type: ReleaseType = ReleaseType.STABLE
        if not self.semver:
            raise Exit(f"KubernetesRelease: could not parse semver from tag '{self.tag}'", code=1)

        # override to RC if applicable
        if self.semver.prerelease and self.semver.prerelease.startswith("rc"):
            self.release_type = ReleaseType.RC

    def as_dict(self):
        return {'tag': self.tag, 'rc': self.release_type == ReleaseType.RC}


# KindKubernetesImage represents a Kubernetes version that has been built and pushed
# to a registry. Accordingly, it will contain an image digest and possibly a kind_version.
class KindKubernetesImage(KubernetesRelease):
    def __init__(self, tag: str, digest: str, kind_version: str = ""):
        super().__init__(tag)

        self.digest: str = digest
        if not self.digest.startswith('sha256:') or len(self.digest) != 71:
            raise Exit(
                f"KindKubernetesImage: invalid digest '{digest}' for tag '{self.tag}' (expected sha256:<64 hex chars>)",
                code=1)

        self.kind_version: str = kind_version

    @classmethod
    def from_dict(cls, data) -> KindKubernetesImage:
        tag = data.get('tag')
        digest = data.get('digest')
        if not tag or not digest:
            raise Exit(
                f"KindKubernetesImage.from_dict: 'tag' and 'digest' are required (got tag={tag!r}, digest={digest!r})",
                code=1,
            )
        kind_version = data.get('kind_version') or ""
        return cls(tag, digest, kind_version)

    def as_dict(self):
        tmp = {'tag': self.tag, 'digest': self.digest, 'rc': self.release_type == ReleaseType.RC, }
        if self.kind_version:
            tmp['kind_version'] = self.kind_version
        return tmp


# KubernetesVersions is a collection of either KubernetesReleases or KindKubernetesImages.
class KubernetesVersions[T: (KubernetesRelease, KindKubernetesImage)]:
    def __init__(self):
        self.versions: list[T] = []

    @classmethod
    def from_dict(cls, data, *, type_: type[T]) -> KubernetesVersions[T]:
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
        """Add a version. If a version with the same tag already exists, replace it."""
        for i, existing in enumerate(self.versions):
            if existing.tag == version.tag:
                self.versions[i] = version
                return
        self.versions.append(version)

    def latest(self, release_type: ReleaseType) -> T | None:
        filtered = [r for r in self.versions if r.release_type == release_type]
        if filtered:
            return max(filtered, key=lambda x: x.semver)
        return None

    def contains(self, tag: str) -> bool:
        return any(x.tag == tag for x in self.versions)


class JobLocation(NamedTuple):
    """Line indices for the parts of a Kubernetes latest-style job. Any field is None if not found."""
    job_start: int | None
    rules_line: int | None
    extra_params_line: int | None
    job_end: int | None


class WhenNeverLocation(NamedTuple):
    rules_line: int
    when_never_line: int | None


class E2EJobFile:
    def __init__(self, file_name: str) -> None:
        self.file_name = file_name

        if not os.path.exists(file_name):
            raise Exit(f"E2EJobFile: file not found: {file_name}", code=1)

        with open(file_name) as f:
            self._lines = f.read().split("\n")

    def get_kubernetes_image(self, job_name: str) -> KindKubernetesImage:
        """Parse and return the current kubernetes image for a job. Re-parses every call."""
        loc = _find_k8s_latest_job(self._lines, job_name)
        if loc.extra_params_line is None:
            raise Exit(f"Job '{job_name}' not found in {self.file_name}", code=1)

        pattern = rf'kubernetesVersion=({K8S_VERSION_PATTERN}(?:@sha256:[a-f0-9]+)?)'
        match = re.search(pattern, self._lines[loc.extra_params_line])
        if not match:
            raise Exit(f"Could not parse kubernetesVersion=... in job '{job_name}' of {self.file_name}", code=1)

        parts = match.group(1).split('@')
        if len(parts) != 2:
            raise Exit(
                f"kubernetesVersion in job '{job_name}' of {self.file_name} must be in <version>@<digest> format",
                code=1)
        version, digest = parts

        return KindKubernetesImage(version, digest)

    def set_kubernetes_image(self, job_name: str, kubernetes_image: KindKubernetesImage) -> None:
        loc = _find_k8s_latest_job(self._lines, job_name)
        if loc.extra_params_line is None:
            raise Exit(f"Job '{job_name}' not found in {self.file_name}", code=1)

        new_line, n = re.subn(
            rf'kubernetesVersion={K8S_VERSION_PATTERN}@sha256:[a-f0-9]+',
            f'kubernetesVersion={kubernetes_image.tag}@{kubernetes_image.digest}',
            self._lines[loc.extra_params_line],
            count=1,
        )
        if n == 0:
            raise Exit(f"Failed to substitute kubernetesVersion in job '{job_name}' of {self.file_name}", code=1)
        self._lines[loc.extra_params_line] = new_line

    def _find_when_never(self, job_name: str) -> WhenNeverLocation:
        """Locate the `when: never` rule (if any) within the job's rules block."""
        loc = _find_k8s_latest_job(self._lines, job_name)
        if loc.rules_line is None or loc.job_end is None:
            raise Exit(f"Job '{job_name}' not found in {self.file_name}", code=1)

        for i, line in enumerate(self._lines[loc.rules_line:loc.job_end], start=loc.rules_line):
            if "when: never" in line:
                return WhenNeverLocation(rules_line=loc.rules_line, when_never_line=i)
        return WhenNeverLocation(rules_line=loc.rules_line, when_never_line=None)

    def enable(self, job_name: str) -> bool:
        """Enable the job. Returns True if the file was mutated."""
        loc = self._find_when_never(job_name)
        if loc.when_never_line is None:
            return False
        del self._lines[loc.when_never_line]
        return True

    def disable(self, job_name: str) -> bool:
        """Disable the job. Returns True if the file was mutated."""
        loc = self._find_when_never(job_name)
        if loc.when_never_line is not None:
            return False  # already disabled

        rules_line = self._lines[loc.rules_line]
        indent = ' ' * (len(rules_line) - len(rules_line.lstrip()) + 2)
        self._lines.insert(loc.rules_line + 1, f"{indent}- when: never # disabled — RC not newer than latest stable")
        return True

    def save(self) -> None:
        with open(self.file_name, 'w') as f:
            f.write('\n'.join(self._lines))


def fetch_github_releases(version_pattern: str = K8S_VERSION_PATTERN) -> KubernetesVersions[KubernetesRelease]:
    """Get releases from Kubernetes GitHub repository."""
    discovered = KubernetesVersions[KubernetesRelease]()

    # Get the latest 25 releases. TODO(TBD): is 25 enough, or should we paginate?
    url = f"{GITHUB_URL_BASE}/repos/kubernetes/kubernetes/releases?per_page=25&page=1"

    try:
        response = requests.get(url, timeout=30)
        response.raise_for_status()

        for release in response.json():
            tag_name = release.get('tag_name', '')
            if re.fullmatch(version_pattern, tag_name):
                discovered.add(KubernetesRelease(tag_name))

    except requests.exceptions.RequestException as e:
        raise Exit(f"Failed to fetch Kubernetes releases from {url}: {e}", code=1) from e

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
    Fetch the latest Kubernetes releases from GitHub. Returns a collection containing
    the latest stable release, and the latest RC only if it is newer than the latest stable.
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


def _find_k8s_latest_job(content: list[str], job_name: str) -> JobLocation:
    """
    Locate a Kubernetes latest-style job (one with a `kubernetesVersion=<tag>@<digest>` line)
    within the given YAML content, by top-level job name.

    job_end is the line index of the next top-level entry. Any field is None if not found.
    """
    in_job = False
    job_start = None
    extra_params_line = None
    rules_line = None

    for i, line in enumerate(content):
        if line.strip().startswith(f'{job_name}:'):
            in_job = True
            job_start = i
            continue

        if in_job:
            if 'rules:' in line:
                rules_line = i

            if 'EXTRA_PARAMS:' in line and 'kubernetesVersion=' in line:
                extra_params_line = i

            if line and not line[0].isspace() and line.strip().endswith(':'):
                return JobLocation(job_start, rules_line, extra_params_line, i)

    # corner case if the yaml block was the last one in the file
    if in_job:
        return JobLocation(job_start, rules_line, extra_params_line, len(content))

    return JobLocation(None, None, None, None)


def _update_e2e_yaml_file(
        k8s_jobs: E2EJobFile,
        job_name: str,
        desired: KindKubernetesImage,
) -> tuple[bool, list[str]]:
    """
    Set the kubernetesVersion of `job_name` to `desired` if it differs from the current value.

    Returns (updated: bool, updated_versions: list[str])
    """
    print(f"Desired version for {job_name}: {desired.tag}")

    current = k8s_jobs.get_kubernetes_image(job_name)
    print(f"Current version in {job_name}: {current.tag}")

    if current.tag == desired.tag and current.digest == desired.digest:
        print(f"{job_name} is already at the desired version")
        return False, []

    k8s_jobs.set_kubernetes_image(job_name, desired)
    print(f"Updated {job_name} to {desired.tag}")
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

    print("Fetching latest Kubernetes version...")
    latest = _get_latest_releases()
    if not latest.versions:
        _set_github_output('has_new_versions', 'false')
        raise Exit("fetch-versions: GitHub returned no Kubernetes releases matching the expected pattern", code=1)

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
        print("\nNo new version(s) found")
        _set_github_output('has_new_versions', 'false')


@task
def update_e2e_yaml(_, versions_file=VERSIONS_FILE):
    """
    Update the Kubernetes latest jobs in .gitlab/test/e2e/e2e.yml to the latest
    known versions from `versions_file`.

    Behavior:
      - The stable job (K8S_LATEST_JOB) is pinned to the latest STABLE release.
      - The RC job (K8S_RC_LATEST_JOB) is pinned to the latest RC release AND
        enabled only when that RC is newer than the latest stable; otherwise
        it is disabled via a `- when: never` rule.

    Args:
        versions_file: Path to the JSON file containing versions (default: k8s_versions.json)

    Outputs (GitHub Actions):
        updated: 'true' if any pinned version was updated
        new_versions: comma-delimited list of newly pinned versions (each in backticks)
    """
    _check_dependencies()

    k8s_jobs = E2EJobFile(E2E_YAML_PATH)

    # Check if there are new versions to process
    if not os.path.exists(versions_file):
        print("No versions file found - nothing to update")
        _set_github_output('updated', 'false')
        return

    # Load new versions
    with open(versions_file) as f:
        all_versions = KubernetesVersions.from_dict(json.load(f), type_=KindKubernetesImage)

    print("Checking for new versions to add to e2e.yml...")

    stable = all_versions.latest(ReleaseType.STABLE)
    rc = all_versions.latest(ReleaseType.RC)

    # Stable job: always attempt to update to the latest stable
    if stable is not None:
        updated_stable, added_stable = _update_e2e_yaml_file(k8s_jobs, K8S_LATEST_JOB, stable)
    else:
        print("No stable version available in new_versions")
        updated_stable, added_stable = False, []

    # RC job: only update + enable if there's an RC newer than the latest stable;
    # otherwise disable it so it doesn't run redundantly.
    if rc and (stable is None or rc.semver > stable.semver):
        updated_rc, added_rc = _update_e2e_yaml_file(k8s_jobs, K8S_RC_LATEST_JOB, rc)
        k8s_jobs.enable(K8S_RC_LATEST_JOB)
    else:
        print(f"No RC newer than latest stable — disabling {K8S_RC_LATEST_JOB}")
        k8s_jobs.disable(K8S_RC_LATEST_JOB)
        updated_rc, added_rc = False, []

    k8s_jobs.save()

    updated = updated_stable or updated_rc
    added_versions = added_stable + added_rc

    if updated:
        version_list = ', '.join(f"`{v}`" for v in added_versions)
        _set_github_output('updated', 'true')
        _set_github_output('new_versions', version_list)
        print(f"\nSuccessfully updated to: {', '.join(added_versions)}")
    else:
        _set_github_output('updated', 'false')
        print("\nNo version updates made")


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

    # Iterate ascending by semver so the highest patch/RC for each minor wins.
    for version in sorted(existing_k8s_versions, key=lambda v: v.semver):
        # Derive minor version key ("1.35" from "v1.35.1")
        minor_key = f"{version.semver.major}.{version.semver.minor}"
        # Resolve kind_version: prefer what's already in kind_versions.json, then
        # the version in k8s_versions.json, then fallback to the latest kind release from GitHub.
        existing = kind_versions.get(minor_key, {})
        kind_version = existing.get('kind_version') or version.kind_version
        if not kind_version:
            if latest_kind_release is None:
                latest_kind_release = _get_latest_kind_release()
            kind_version = latest_kind_release
            if not kind_version:
                raise Exit(
                    f"Could not determine kind version for {version.tag} (no kind_version in {KIND_VERSIONS_JSON_PATH} or {VERSIONS_FILE}, and fetching the latest kind release from GitHub failed)",
                    code=1)
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
            raise Exit(
                f"save-versions: --versions is not valid JSON ({e}); expected a JSON object mapping tag to version data",
                code=1) from e

    # Load existing versions
    existing_versions = _load_existing_versions(versions_file)

    for new in versions:
        existing_versions.add(new)

    # Save to file
    _save_versions(existing_versions, versions_file)
