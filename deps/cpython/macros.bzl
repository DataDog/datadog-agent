"""Macros generating the per-module targets of the CPython build."""

load("@@//bazel/rules/dd_packaging:dd_cc_packaged.bzl", "dd_cc_packaged")
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

def cpython_shared_modules(name, modules, copts, ext_suffix, dest_dir):
    """Creates the module_<name> extension module targets and their module_<name>_pkg packaging.

    The name target groups the native_module_<name>_pkg targets.

    Args:
        name: name of the group target.
        modules: dict of module name to its srcs, defines, linkopts, deps and dynamic_deps.
        copts: compiler options for the module sources.
        ext_suffix: the extension module file suffix.
        dest_dir: the directory the modules are installed in.
    """
    for module_name, module in modules.items():
        cc_library(
            name = "native_module_{}_lib".format(module_name),
            srcs = module["srcs"],
            copts = copts,
            linkopts = module.get("linkopts", []),
            local_defines = ["NDEBUG"] + module.get("defines", []),
            tags = ["manual"],
            deps = [":native_internal_headers"] + module.get("deps", []),
        )

        cc_shared_library(
            name = "native_module_" + module_name,
            dynamic_deps = module.get("dynamic_deps", []),
            shared_lib_name = module_name + ext_suffix,
            deps = [":native_module_{}_lib".format(module_name)],
        )

        dd_cc_packaged(
            name = "native_module_{}_pkg".format(module_name),
            dest_dir = dest_dir,
            input = ":native_module_" + module_name,
            mode = "0755",
        )

    pkgs = [":native_module_{}_pkg".format(module_name) for module_name in modules]
    native.filegroup(
        name = name,
        srcs = pkgs,
        data = pkgs,
    )
