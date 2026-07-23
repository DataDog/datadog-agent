load("@rules_go//go:def.bzl", "go_test")
load("//bazel/flavors:defs.bzl", "ALL_FLAVORS", "flavor_gotags")

# OSes each flavor doesn't ship on. Their go_test variant is marked
# target_compatible_with incompatible there, so `bazel test //...`
# doesn't build/run tests for a nonsensical combination.
_FLAVOR_EXCLUDED_OS = {
    "dogstatsd": ["windows"],
    "heroku": ["macos", "windows"],
    "iot": ["macos", "windows"],
}

def _target_compatible_with(flavor, user_tcw):
    excluded_os = _FLAVOR_EXCLUDED_OS.get(flavor, [])
    if not excluded_os:
        return user_tcw
    conditions = {"//conditions:default": []}
    for os_name in excluded_os:
        conditions["@platforms//os:" + os_name] = ["@platforms//:incompatible"]
    return user_tcw + select(conditions)

def dd_agent_go_test(name, flavors = None, tags = None, target_compatible_with = None, **kwargs):
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
        target_compatible_with: Optional user-supplied target_compatible_with;
              merged with the per-flavor platform restrictions this macro adds.
              Declared explicitly for the same reason as `tags`.
        **kwargs: Remaining attrs forwarded to each go_test (srcs, embed, deps, …).
    """
    if flavors == None:
        flavors = ALL_FLAVORS
    user_tags = tags or []
    user_tcw = [] if target_compatible_with == None else target_compatible_with
    for flavor in flavors:
        go_test(
            name = name + "_" + flavor,
            gotags = flavor_gotags(flavor),
            tags = user_tags + ["dd_agent_go_test", "flavor_" + flavor],
            target_compatible_with = _target_compatible_with(flavor, user_tcw),
            **kwargs
        )
