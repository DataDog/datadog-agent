"""Bazel rules for rtloader tests."""

load("@rules_go//go:def.bzl", "go_library", "go_test")

def rtloader_test_go_library(**kwargs):
    """A go_library wrapper that adds the rtloader library.

    Args:
      **kwargs: arguments to be forwarded to go_library
    """
    cdeps = kwargs.pop("cdeps", []) + ["//rtloader:rtloader_dynamic"]

    go_library(
        cdeps = cdeps,
        # WIP: support for windows to come later
        target_compatible_with = select({
            "@platforms//os:linux": [],
            "@platforms//os:macos": [],
            "@platforms//os:windows": ["@platforms//:incompatible"],
        }),
        testonly = True,
        **kwargs
    )

def rtloader_go_test(**kwargs):
    """A go_test wrapper for rtloader tests that sets up the all the necessary dynamic libraries and runtime context.

    Args:
      **kwargs: arguments to be forwarded to go_test
    """
    env = {
        "LD_LIBRARY_PATH": ".",
        "PYTHON_BIN": "$(rootpath @cpython//:python3_bin)",
        "PYTHONPATH": "test/python",
    }
    env.update(kwargs.pop("env", {}))
    data = kwargs.pop("data", []) + [
        "//rtloader:datadog-agent-three",
        "@cpython//:python_unix",
        "@cpython//:python3_bin",
        "//rtloader/test:python_stubs",
    ]

    go_test(
        # Running from the folder where the "three" library we want to dlopen simplifies finding it
        # at runtime for macos, where we can't modify the library search path as easily.
        rundir = "rtloader",
        data = data,
        env = env,
        # WIP: support for windows to come later
        target_compatible_with = select({
            "@platforms//os:linux": [],
            "@platforms//os:macos": [],
            "@platforms//os:windows": ["@platforms//:incompatible"],
        }),
        **kwargs
    )
