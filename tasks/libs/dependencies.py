import os
import sys

from invoke import Exit

from tasks.libs.releasing.json import load_release_json
from tasks.libs.releasing.version import RELEASE_JSON_DEPENDENCIES


def load_overridden_dependencies():
    """
    Load dependency versions from release.json with environment variable overrides.
    WINDOWS_* dependencies are skipped on non-Windows platforms.

    Returns:
        dict: Environment dictionary with dependency versions as strings.
    """
    release = load_release_json()
    if not (dependencies := release.get(RELEASE_JSON_DEPENDENCIES)):
        raise Exit(f"Could not find {RELEASE_JSON_DEPENDENCIES!r} in release.json")
    env = {}
    for key, value in dependencies.items():
        if key.startswith("WINDOWS_") and sys.platform != "win32":
            print(f"Ignoring {key!r} on {sys.platform}", file=sys.stderr)
            continue
        if override := os.getenv(key):
            print(f"Overriding {key!r}: {value!r} -> {override!r}", file=sys.stderr)
            value = override
        # windows runners don't accept anything else than strings in the environment when running a subprocess.
        env[key] = str(value)
    return env
