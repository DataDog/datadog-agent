"""Macros generating the per-module targets of the CPython build."""

load("@bazel_lib//lib:run_binary.bzl", "run_binary")

def cpython_frozen_modules(modules, tool):
    """Creates a frozen_<name> header target for each frozen module.

    Args:
        modules: list of (name, source) pairs.
        tool: the _freeze_module binary.
    """
    for name, src in modules:
        run_binary(
            name = "native_frozen_" + name,
            srcs = [src],
            outs = ["Python/frozen_modules/{}.h".format(name)],
            args = [
                name,
                "$(execpath {})".format(src),
                "$@",
            ],
            tags = ["manual"],
            tool = tool,
        )
