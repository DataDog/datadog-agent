load("@rules_go//go:def.bzl", "go_test")
load(
    "//bazel/test_tags:defs.bzl",
    "test_tag_set_suffix",
    "test_tag_set_tags",
    "test_tag_set_target_compatible_with",
)

def dd_agent_go_test(
        name,
        tag_sets = None,
        include_default = True,
        tags = None,
        target_compatible_with = None,
        **kwargs):
    """Wraps go_test with a default target and relevant tag-set variants.

    Args:
        name: Default target name and prefix for tag-set variants.
        tag_sets: Encoded tag combinations, such as "zlib+zstd".
        include_default: Whether to emit the minimally tagged default test.
        tags: Optional user-supplied Bazel tags.
        target_compatible_with: Optional user-supplied target_compatible_with;
              merged with tag-set platform restrictions.
        **kwargs: Remaining attrs forwarded to each go_test (srcs, embed, deps, …).
    """
    user_tags = tags or []
    user_tcw = [] if target_compatible_with == None else target_compatible_with

    if include_default:
        go_test(
            name = name,
            gotags = test_tag_set_tags(),
            tags = user_tags + ["dd_agent_go_test"],
            target_compatible_with = user_tcw,
            **kwargs
        )

    for tag_set in tag_sets or []:
        go_test(
            name = name + "_" + test_tag_set_suffix(tag_set),
            gotags = test_tag_set_tags(tag_set),
            tags = user_tags + ["dd_agent_go_test", "tagset_" + test_tag_set_suffix(tag_set)],
            target_compatible_with = user_tcw + test_tag_set_target_compatible_with(tag_set),
            **kwargs
        )
