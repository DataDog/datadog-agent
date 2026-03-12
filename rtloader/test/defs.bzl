"""Bazel rules for rtloader tests."""

load("@rules_go//go:def.bzl", "go_library", "go_test")

def rtloader_test_go_library(**kwargs):
    """A go_library wrapper that adds the rtloader library.

    Args:
      **kwargs: arguments to be forwarded to go_library
    """

    go_library(
        cdeps = kwargs.pop("cdeps", []) + select({
            "@platforms//os:linux": ["//rtloader:rtloader_dynamic"],
            "@platforms//os:macos": ["//rtloader:rtloader_dynamic"],
            "@platforms//os:windows": ["//rtloader:rtloader"],
        }),
        copts = kwargs.pop("copts", []) + ["-Wno-deprecated-declarations"],
        testonly = True,
        **kwargs
    )

def rtloader_go_test(**kwargs):
    """A go_test wrapper for rtloader tests that sets up the all the necessary dynamic libraries and runtime context.

    Args:
      **kwargs: arguments to be forwarded to go_test
    """
    unix_data = [
        "//rtloader:datadog-agent-three",
        "@cpython//:python_unix",
        "@cpython//:python3_bin",
    ]

    unix_env = {
        "LD_LIBRARY_PATH": ".",
        "PYTHON_BIN": "$(rlocationpath @cpython//:python3_bin)",
    }

    go_test(
        # Running from the folder where the "three" library we want to dlopen simplifies finding it
        # at runtime for macos, where we can't modify the library search path as easily.
        # Note: this has no effect on windows
        rundir = "rtloader",
        data = kwargs.pop("data", []) +
               ["//rtloader/test:dir_with_python_stubs"] +
               select({
                   "@platforms//os:linux": unix_data,
                   "@platforms//os:macos": unix_data,
                   "@platforms//os:windows": [
                       "//rtloader/test:dir_with_three",
                       "@cpython//:python_win",
                       "@cpython//:python_win_lib",
                   ],
               }),
        env = kwargs.pop("env", {}) |
              {"STUBS_LOCATION": "$(rlocationpath //rtloader/test:dir_with_python_stubs)"} |
              select({
                  "@platforms//os:linux": unix_env,
                  "@platforms//os:macos": unix_env,
                  "@platforms//os:windows": {
                      "THREE_PATH": "$(rlocationpath //rtloader/test:dir_with_three)",
                      "PYTHON_LIB": "$(rlocationpath @cpython//:python_win_lib)",
                  },
              }),
        **kwargs
    )
