"""Version information from release.json for Windows resource compilation."""

load("@dd_release_json//:release_json.bzl", "release_json")

def agent_version_defines():
    """Returns a dict of -D defines for windres, matching tasks/windows_resources.py:versioninfo_vars()."""
    milestone = release_json.get("current_milestone")
    parts = milestone.split(".")
    return {
        "MAJ_VER": parts[0],
        "MIN_VER": parts[1],
        "PATCH_VER": parts[2],
        "PY3_RUNTIME": "1",
        "BUILD_ARCH_x64": "1",
    }
