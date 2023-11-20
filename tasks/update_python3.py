import re
from collections import defaultdict

import requests
import semver
from invoke import task
from invoke.exceptions import Exit

AVAILABLE_PYTHON3_VERSIONS = "https://raw.githubusercontent.com/actions/python-versions/main/versions-manifest.json"
PYTHON3_VERSION_FILE = "./.python3-version"
GHA_SUPPORTED_PLATFORMS = [("win32", "x64"), ("darwin", "x64"), ("darwin", "arm64")]
# From https://semver.org/
SEMVER_REGEX = r"^(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)(?:-(?P<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+(?P<buildmetadata>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$"
MAJOR_MINOR_REGEX = r"^(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)"


@task
def python3_version(_):
    """
    Get the python3 version used in the Github Actions Workflow stored in PYTHON3_VERSION_FILE.
    """
    current_version = _get_repo_python3_version()
    print(current_version)


@task
def update_python3_gha(_, version: str = None, show_available=False):
    """
    Updates the python3 version used in the Github Actions Workflow.
    By modifying the PYTHON3_VERSION_FILE with a version that's available for all the used platforms.

    If you give a full version e.g 3.11.2 it will try to bump to the specific version i.e 3.11.2
    If you give a major version e.g 3.11 it will display the available versions available i.e ['3.11.1', '3.11.2', '3.11.3']
    """
    available_versions = list_available_versions(GHA_SUPPORTED_PLATFORMS, keep_stable_only=True)
    if show_available or not version:
        print(f"Available Python versions for platforms {GHA_SUPPORTED_PLATFORMS}:\n", available_versions)
        return
    is_full_version = re.match(SEMVER_REGEX, version)

    # If version is a full version
    if is_full_version:
        if version not in available_versions:
            raise Exit(
                f"Python {version} is not available for all the following platforms\n{GHA_SUPPORTED_PLATFORMS}", code=1
            )

    # If version is a major version then bump to the latest stable version
    if not is_full_version:
        filtered_versions = filter_versions(available_versions, majmin=version)
        if not filtered_versions:
            raise Exit(
                f"No {version} version of Python is available for all the following platforms\n{GHA_SUPPORTED_PLATFORMS}",
                code=1,
            )
        print(
            f"List of available {version} Python versions for the platforms {GHA_SUPPORTED_PLATFORMS}:\n",
            filtered_versions,
        )
        return

    print(f"Bumping Python from {_get_repo_python3_version()} to {version} !")
    _update_python3_version_file(version)
    print("Done !")


def _get_repo_python3_version() -> str:
    with open(PYTHON3_VERSION_FILE, "r", encoding="utf-8") as f:
        version = f.read()
    return version.strip()


def _update_python3_version_file(version: str):
    with open(PYTHON3_VERSION_FILE, "w", encoding="utf-8") as f:
        f.write(version)


def filter_versions(versions: list[str], majmin: str = None, keep_stable: bool = False) -> list[str]:
    """
    Keeps only stable versions from a version list.
    E.g ['3.13.0-alpha.1', '3.12.0', '3.12.0-rc.3', '3.11.5', '3.10.11']
    becomes ['3.12.0', '3.11.5', '3.10.11'].

    majmin[str]: Keep the versions having this majmin e.g 3.9 will keep 3.9.11, 3.9.1, ...

    keep_stable[bool]: Keep the stable versions i.e neither the 3.13.0-alpha.1 nor the 3.12.0-rc.3

    """
    if not keep_stable and not majmin:
        return versions

    if majmin:
        majmin_check = re.match(MAJOR_MINOR_REGEX, majmin)
        semver_decomposition = re.match(SEMVER_REGEX, majmin)
        if not majmin_check:
            raise ValueError(f"The majmin '{majmin}' format should be x.y")
        if semver_decomposition:
            raise ValueError(f"The majmin argument's format should be x.y, '{majmin}' is x.y.z")

    filtered_versions = []
    for v in versions:
        v_info = semver.VersionInfo.parse(v)
        keeping_version = True
        if keep_stable and (v_info.prerelease or v_info.build):
            keeping_version = False
        if majmin and f"{v_info.major}.{v_info.minor}" != majmin:
            keeping_version = False
        if keeping_version:
            filtered_versions.append(v)

    filtered_versions.reverse()

    return filtered_versions


def max_version(versions: list[str]) -> str:
    """
    Returns the semver maximum of a versions list.
    E.g ['3.12.0', '3.12.0-rc.3', '3.11.5', '3.10.11'] -> '3.12.0-rc.3'
    E.g ['3.12.0', '3.2.0'] -> '3.12.0'
    """
    if not versions:
        raise ValueError("Can't determine the maximum version of an empty versions list.")
    return str(max(map(semver.VersionInfo.parse, versions)))


def list_available_versions(platforms: list[tuple] = None, keep_stable_only=False) -> list[str]:
    """
    This functions fetches AVAILABLE_PYTHON3_VERSIONS and list all versions available on a set of os x arch.

    Example: list_available_versions(platforms=[("win32", "x64"), ("darwin", "x64"), ("darwin", "arm64")])
    """

    platforms = GHA_SUPPORTED_PLATFORMS if not platforms else platforms

    req = requests.get(AVAILABLE_PYTHON3_VERSIONS, allow_redirects=True, timeout=5)
    if req.status_code >= 400:
        raise Exit(
            "Couldn't fetch the versions-manifest.json file from {AVAILABLE_PYTHON3_VERSIONS} (status code: {req.status_code})."
        )
    python3_versions = req.json()

    available_platforms = defaultdict(list)
    for elt in python3_versions:
        available_platforms[elt["version"]] = [(file["platform"], file["arch"]) for file in elt["files"]]
    available_versions = [
        version for version in available_platforms if set(platforms) <= set(available_platforms[version])
    ]
    return filter_versions(available_versions, keep_stable=keep_stable_only)
