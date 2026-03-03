import os
import sys

from invoke import Exit

from tasks.libs.releasing.json import load_release_json
from tasks.libs.releasing.version import RELEASE_JSON_DEPENDENCIES


def get_effective_dependencies_env():
    """
    Load dependency versions from release.json with environment variable overrides.
    WINDOWS_* dependencies are skipped on non-Windows platforms.

    Returns:
        dict: Environment dictionary with dependency versions as strings.
    """
    release = load_release_json()
    if not (release_dependencies := release.get(RELEASE_JSON_DEPENDENCIES)):
        raise Exit(f"Could not find {RELEASE_JSON_DEPENDENCIES!r} in release.json")
    effective_dependencies_env = {}
    for key, value in release_dependencies.items():
        if key.startswith("WINDOWS_") and sys.platform != "win32":
            print(f"Ignoring {key!r} on {sys.platform}", file=sys.stderr)
            continue
        if override := os.getenv(key):
            print(f"Overriding {key!r}: {value!r} -> {override!r}", file=sys.stderr)
            value = override
        # windows runners don't accept anything else than strings in the environment when running a subprocess.
        effective_dependencies_env[key] = str(value)
    return effective_dependencies_env
