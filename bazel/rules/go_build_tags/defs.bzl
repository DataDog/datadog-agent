load("@rules_go//go:def.bzl", "go_test")
load("//bazel/flavors:defs.bzl", "flavor_gotags")

def dd_go_test(name, flavors, **kwargs):
    """Wraps go_test with per-flavor variants grouped under a test_suite.

    The flavor-to-gotags mapping and the tag naming scheme are encapsulated
    here so that BUILD files only express intent (which flavors apply) and
    any future change to how flavors are implemented only requires updating
    this macro, not every BUILD file.

    Args:
        name: Base name; used for the test_suite and as the prefix for each
              per-flavor go_test (e.g. "foo_test_base", "foo_test_iot").
        flavors: List of flavor names this package is built and tested under.
        **kwargs: Remaining attrs forwarded to each go_test (srcs, embed, deps, …).
    """
    for flavor in flavors:
        go_test(
            name = name + "_" + flavor,
            gotags = flavor_gotags(flavor),
            tags = ["go_tests", "flavor_" + flavor],
            **kwargs
        )
    native.test_suite(
        name = name,
        tests = [":" + name + "_" + flavor for flavor in flavors],
    )
