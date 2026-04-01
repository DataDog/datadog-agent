load("@rules_go//go:def.bzl", "go_library")

def go_testutil(name, srcs = [], embed = [], deps = [], visibility = None):
    """A test-only go_library for test utilities shared across packages.

    Files with //go:build test that are needed by tests outside the package
    are placed in a go_testutil target instead of go_test.srcs. The target
    embeds the main library and adds the test-gated files, making the combined
    package available to other packages' test targets.
    """
    go_library(
        name = name,
        srcs = srcs,
        embed = embed,
        deps = deps,
        testonly = True,
        visibility = visibility if visibility != None else ["//visibility:public"],
    )
