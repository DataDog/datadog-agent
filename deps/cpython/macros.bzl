"""Macros generating the per-module targets of the CPython build."""

load("@bazel_lib//lib:run_binary.bzl", "run_binary")
load("@rules_cc//cc:defs.bzl", "cc_library", "cc_shared_library")

def cpython_frozen_modules(modules, tool):
    """Creates a frozen_<name> header target for each frozen module.

    Args:
        modules: list of (name, source) pairs.
        tool: the _freeze_module binary.
    """
    for name, src in modules:
        run_binary(
            name = "frozen_" + name,
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

def cpython_shared_modules(modules, copts, ext_suffix, target_compatible_with):
    """Creates the module_<name> extension module targets.

    Args:
        modules: dict of module name to its srcs, defines, linkopts, deps and dynamic_deps.
        copts: compiler options for the module sources.
        ext_suffix: the extension module file suffix.
        target_compatible_with: platforms the modules are built for.

    Returns:
        The labels of the module_<name> targets.
    """
    for name, module in modules.items():
        cc_library(
            name = "module_{}_lib".format(name),
            srcs = module["srcs"],
            copts = copts,
            linkopts = module.get("linkopts", []),
            local_defines = ["NDEBUG"] + module.get("defines", []),
            tags = ["manual"],
            target_compatible_with = target_compatible_with,
            deps = [":internal_headers"] + module.get("deps", []),
        )

        cc_shared_library(
            name = "module_" + name,
            dynamic_deps = module.get("dynamic_deps", []),
            shared_lib_name = name + ext_suffix,
            target_compatible_with = target_compatible_with,
            user_link_flags = select({
                "@platforms//os:macos": [
                    "-undefined",
                    "dynamic_lookup",
                ],
                "//conditions:default": [],
            }),
            deps = [":module_{}_lib".format(name)],
        )

    return [":module_" + name for name in modules]
