"""Helpers for flavorless Go test tag-set variants."""

load(
    "//tasks:build_tags.bzl",
    "AIX_EXCLUDED_TAGS",
    "BASE_TEST_TAGS",
    "DARWIN_EXCLUDED_TAGS",
    "LINUX_ONLY_TAGS",
    "WINDOWS_EXCLUDED_TAGS",
    "WINDOWS_INCLUDED_TAGS",
)

def test_tag_set_tags(tag_set = None):
    """Returns build tags for the default test or an encoded tag set.

    Args:
        tag_set: An optional plus-delimited build tag set.

    Returns:
        The sorted build tags for the test variant.
    """
    if tag_set == None:
        return BASE_TEST_TAGS
    return sorted(set(BASE_TEST_TAGS) | set(tag_set.split("+")))

def test_tag_set_suffix(tag_set):
    """Returns a target-name-safe suffix for an encoded tag set.

    Args:
        tag_set: A plus-delimited build tag set.

    Returns:
        A target-name-safe suffix.
    """
    return tag_set.replace(".", "_").replace("+", "_")

def test_tag_set_target_compatible_with(tag_set):
    """Returns platform restrictions implied by an encoded tag set.

    Args:
        tag_set: An optional plus-delimited build tag set.

    Returns:
        A target_compatible_with value for the test variant.
    """
    if tag_set == None:
        return []

    tags = set(tag_set.split("+"))
    incompatible = []
    if tags & LINUX_ONLY_TAGS:
        incompatible.extend(["macos", "windows"])
    if tags & WINDOWS_INCLUDED_TAGS:
        incompatible.extend(["linux", "macos"])
    if tags & WINDOWS_EXCLUDED_TAGS:
        incompatible.append("windows")
    if tags & DARWIN_EXCLUDED_TAGS:
        incompatible.append("macos")
    if tags & AIX_EXCLUDED_TAGS:
        incompatible.append("aix")

    if not incompatible:
        return []

    conditions = {"//conditions:default": []}
    for os_name in incompatible:
        conditions["@platforms//os:" + os_name] = ["@platforms//:incompatible"]
    return select(conditions)
