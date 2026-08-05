"""Helpers for flavorless Go test gotags-set variants."""

load(
    "//tasks:build_tags.bzl",
    "AIX_EXCLUDED_TAGS",
    "BASE_TEST_TAGS",
    "DARWIN_EXCLUDED_TAGS",
    "LINUX_ONLY_TAGS",
    "WINDOWS_EXCLUDED_TAGS",
    "WINDOWS_INCLUDED_TAGS",
)

# Short target-name suffixes for gotags sets whose joined form is too long to fit in
# a Windows runfiles path (see test_tag_set_check_name). Only needed for sets
# that build on Windows.
_TAG_SET_SUFFIX_ALIASES = {
    "cel+clusterchecks+kubeapiserver+kubelet+orchestrator": "dca",
    "cel+clusterchecks+docker+kubeapiserver+kubelet+orchestrator": "dca_docker",
}

# Windows caps a process's current directory at MAX_PATH even where longer paths
# are otherwise allowed, and Bazel's test wrapper chdirs into
# <name>_/<name>.exe.runfiles (tools/test/windows/tw.cc) before spawning the test
# binary — so a name is spent twice and an over-long one fails at test time, not
# at build time. The budget is measured against the CI execroot layout
# C:\bob\execroot\_main\bazel-out\x64_windows-fastbuild-ST-<12 hex>\bin.
_WINDOWS_MAX_PATH = 260
_WINDOWS_BIN_PREFIX_LEN = 73

# Separators plus the "_" and ".exe.runfiles\_main" the wrapper appends.
_WINDOWS_RUNFILES_OVERHEAD = 23

def _tag_set_key(gotags):
    return "+".join(sorted(gotags))

def _excluded_os(gotags):
    """Returns the OS names a gotags set cannot be built for.

    Args:
        gotags: A list of Go build tags.

    Returns:
        A list of @platforms//os names.
    """
    tags = set(gotags)
    excluded = []
    if tags & LINUX_ONLY_TAGS:
        excluded.extend(["macos", "windows"])
    if tags & WINDOWS_INCLUDED_TAGS:
        excluded.extend(["linux", "macos"])
    if tags & WINDOWS_EXCLUDED_TAGS:
        excluded.append("windows")
    if tags & DARWIN_EXCLUDED_TAGS:
        excluded.append("macos")
    if tags & AIX_EXCLUDED_TAGS:
        excluded.append("aix")
    return excluded

def test_tag_set_tags(gotags = None):
    """Returns build tags for the default test or a gotags set.

    Args:
        gotags: An optional list of Go build tags.

    Returns:
        The platform-aware build tags for the test variant.
    """
    if gotags == None:
        tags = BASE_TEST_TAGS
    else:
        tags = sorted(set(BASE_TEST_TAGS) | set(gotags))

    # Windows builds always define WINDOWS_INCLUDED_TAGS, as
    # filter_incompatible_tags() does in tasks/build_tags.py. Without them,
    # sources gated on `windows && wmi` drop out while the tests covering them
    # still compile. Tags a set cannot be built with on a given OS need no
    # filtering here: test_tag_set_target_compatible_with() already makes the
    # variant incompatible there.
    return select({
        "@platforms//os:windows": sorted(set(tags) | WINDOWS_INCLUDED_TAGS),
        "//conditions:default": tags,
    })

def test_tag_set_suffix(gotags):
    """Returns a target-name-safe suffix for a gotags set.

    Args:
        gotags: A list of Go build tags.

    Returns:
        A target-name-safe suffix.
    """
    key = _tag_set_key(gotags)
    alias = _TAG_SET_SUFFIX_ALIASES.get(key)
    if alias:
        return alias
    return "_".join(gotags)

def test_tag_set_check_name(name, gotags = None):
    """Fails if a test target's Windows runfiles path would exceed MAX_PATH.

    Args:
        name: The generated target name.
        gotags: An optional list of Go build tags.
    """
    if gotags != None and "windows" in _excluded_os(gotags):
        return

    length = _WINDOWS_BIN_PREFIX_LEN + len(native.package_name()) + 2 * len(name) + _WINDOWS_RUNFILES_OVERHEAD
    if length > _WINDOWS_MAX_PATH:
        fail(
            ("test target %s would need a %d-character Windows runfiles path (max %d), so it " +
             "cannot run there. Shorten the target name, or give its gotags set a short suffix in " +
             "_TAG_SET_SUFFIX_ALIASES in //bazel/test_tags:defs.bzl.") % (name, length, _WINDOWS_MAX_PATH),
        )

def test_tag_set_target_compatible_with(gotags):
    """Returns platform restrictions implied by a gotags set.

    Args:
        gotags: A list of Go build tags.

    Returns:
        A target_compatible_with value for the test variant.
    """
    if gotags == None:
        return []

    excluded = _excluded_os(gotags)
    if not excluded:
        return []

    conditions = {"//conditions:default": []}
    for os_name in excluded:
        conditions["@platforms//os:" + os_name] = ["@platforms//:incompatible"]
    return select(conditions)
