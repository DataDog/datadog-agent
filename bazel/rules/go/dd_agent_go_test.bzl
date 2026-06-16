load("@rules_go//go:def.bzl", "go_test")
load("//bazel/flavors:defs.bzl", "ALL_FLAVORS", "flavor_gotags")

def dd_agent_go_test(name, flavors = None, tags = None, **kwargs):
    """Wraps go_test with per-flavor variants.

    The flavor-to-gotags mapping and the tag naming scheme are encapsulated
    here so that BUILD files only express intent (which flavors apply) and
    any future change to how flavors are implemented only requires updating
    this macro, not every BUILD file.

    Args:
        name: Base name; used as the prefix for each per-flavor go_test
              (e.g. "foo_test_base", "foo_test_iot").
        flavors: List of flavor names to test under. Defaults to all flavors.
                 Override to restrict testing to a subset.
        tags: Optional user-supplied bazel tags; merged with the per-flavor
              tags this macro adds. Declared explicitly (rather than left in
              **kwargs) so passing it doesn't collide with the macro's own
              `tags=` on each underlying go_test.
        **kwargs: Remaining attrs forwarded to each go_test (srcs, embed, deps, …).
    """
    if flavors == None:
        flavors = ALL_FLAVORS
    user_tags = tags or []
    for flavor in flavors:
        go_test(
            name = name + "_" + flavor,
            gotags = flavor_gotags(flavor),
            tags = user_tags + ["dd_agent_go_test", "flavor_" + flavor],
            **kwargs
        )
