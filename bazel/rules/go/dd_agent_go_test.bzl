load("@rules_go//go:def.bzl", "go_test")
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
# a Windows runfiles path (see _test_tag_set_check_name). Only needed for sets
# that build on Windows.
_TAG_SET_SUFFIX_ALIASES = {
    "cel+clusterchecks+kubeapiserver+kubelet+orchestrator": "dca",
    "cel+clusterchecks+docker+kubeapiserver+kubelet+orchestrator": "dca_docker",
    "cel+clusterchecks+containerd+docker+kubeapiserver+kubelet+orchestrator": "dca_containerd_docker",
    "cel+clusterchecks+docker+kubeapiserver+kubelet+orchestrator+python": "dca_docker_python",
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

def _test_tag_set_tags(gotags = None):
    if gotags == None:
        tags = BASE_TEST_TAGS
    else:
        tags = sorted(set(BASE_TEST_TAGS) | set(gotags))

    # Windows builds always define WINDOWS_INCLUDED_TAGS, as
    # filter_incompatible_tags() does in tasks/build_tags.py. Without them,
    # sources gated on `windows && wmi` drop out while the tests covering them
    # still compile.
    return select({
        "@platforms//os:windows": sorted(set(tags) | WINDOWS_INCLUDED_TAGS),
        "//conditions:default": tags,
    })

def _test_tag_set_suffix(gotags):
    key = _tag_set_key(gotags)
    alias = _TAG_SET_SUFFIX_ALIASES.get(key)
    if alias:
        return alias
    return "_".join(gotags)

def _test_tag_set_check_name(name, gotags = None):
    if gotags != None and "windows" in _excluded_os(gotags):
        return

    length = _WINDOWS_BIN_PREFIX_LEN + len(native.package_name()) + 2 * len(name) + _WINDOWS_RUNFILES_OVERHEAD
    if length > _WINDOWS_MAX_PATH:
        fail(
            ("test target %s would need a %d-character Windows runfiles path (max %d), so it " +
             "cannot run there. Shorten the target name, or give its gotags set a short suffix in " +
             "_TAG_SET_SUFFIX_ALIASES in //bazel/rules/go:dd_agent_go_test.bzl.") % (name, length, _WINDOWS_MAX_PATH),
        )

def _test_tag_set_target_compatible_with(gotags):
    if gotags == None:
        return []

    excluded = _excluded_os(gotags)
    if not excluded:
        return []

    conditions = {"//conditions:default": []}
    for os_name in excluded:
        conditions["@platforms//os:" + os_name] = ["@platforms//:incompatible"]
    return select(conditions)

def dd_agent_go_test(
        name,
        gotags_sets = None,
        include_default = True,
        tags = None,
        target_compatible_with = None,
        **kwargs):
    """Wraps go_test with a default target and relevant gotags-set variants.

    Args:
        name: Default target name and prefix for gotags-set variants.
        gotags_sets: Lists of Go build tags, such as [["zlib", "zstd"]].
        include_default: Whether to emit the minimally tagged default test.
        tags: Optional user-supplied Bazel tags.
        target_compatible_with: Optional user-supplied target_compatible_with;
              merged with gotags-set platform restrictions.
        **kwargs: Remaining attrs forwarded to each go_test (srcs, embed, deps, …).
    """
    user_tags = tags or []
    user_tcw = [] if target_compatible_with == None else target_compatible_with

    if include_default:
        _test_tag_set_check_name(name)
        go_test(
            name = name,
            gotags = _test_tag_set_tags(),
            tags = user_tags + ["dd_agent_go_test"],
            target_compatible_with = user_tcw,
            **kwargs
        )

    for gotags in gotags_sets or []:
        suffix = _test_tag_set_suffix(gotags)
        _test_tag_set_check_name(name + "_" + suffix, gotags)
        go_test(
            name = name + "_" + suffix,
            gotags = _test_tag_set_tags(gotags),
            tags = user_tags + ["dd_agent_go_test", "tagset_" + suffix],
            target_compatible_with = user_tcw + _test_tag_set_target_compatible_with(gotags),
            **kwargs
        )
