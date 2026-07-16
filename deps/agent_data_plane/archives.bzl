"""Per-platform selection of the prebuilt agent-data-plane archive."""

_ARCHIVE_BY_CONDITION = {
    "//packages/agent:linux_x86_64_fips": "agent_data_plane_fips_linux_amd64",
    "//packages/agent:linux_arm64_fips": "agent_data_plane_fips_linux_arm64",
    "//packages/agent:windows_x86_64_fips": "agent_data_plane_fips_windows_amd64",
    "//:linux_x86_64": "agent_data_plane_linux_amd64",
    "//:linux_arm64": "agent_data_plane_linux_arm64",
    "//:macos_x86_64": "agent_data_plane_darwin_amd64",
    "//:macos_arm64": "agent_data_plane_darwin_arm64",
    "//:windows_x86_64": "agent_data_plane_windows_amd64",
}

def _archive_label(repo, target):
    return "@{}//:{}".format(repo, target)

def adp_archive_targets(target):
    """A select() resolving to `["@<archive>//:<target>"]` for the current platform/flavor.

    Args:
        target: name of the target within the chosen archive repo.

    Returns:
        A select() value usable as a `label_list`-typed attribute.
    """
    return select({
        condition: [_archive_label(repo, target)]
        for condition, repo in _ARCHIVE_BY_CONDITION.items()
    })

def adp_archive_renames(target, new_name):
    """A select() resolving to `{"@<archive>//:<target>": new_name}` for the current platform/flavor.

    Usable directly as a `pkg_files` `renames` value, to rename one specific
    file from the archive selected for the current platform/flavor.

    Args:
        target: name of the target within the chosen archive repo.
        new_name: destination name to rename that target's file to.

    Returns:
        A select() value usable as a `renames`-typed attribute.
    """
    return select({
        condition: {_archive_label(repo, target): new_name}
        for condition, repo in _ARCHIVE_BY_CONDITION.items()
    })
