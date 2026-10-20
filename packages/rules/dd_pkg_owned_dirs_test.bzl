"""Tests for dd_pkg_owned_dirs.bzl."""

load("@rules_pkg//pkg:mappings.bzl", "pkg_filegroup", "pkg_files", "pkg_mkdirs", "pkg_mklink")
load("@rules_pkg//pkg:providers.bzl", "PackageDirsInfo")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:util.bzl", "util")
load(":dd_pkg_owned_dirs.bzl", "dd_pkg_owned_dirs")

def _tree_impl(ctx):
    out = ctx.actions.declare_directory(ctx.label.name)
    ctx.actions.run_shell(outputs = [out], command = "true")
    return [DefaultInfo(files = depset([out]))]

_tree = rule(implementation = _tree_impl)

def _test_parent_dirs(name):
    util.helper_target(
        pkg_files,
        name = name + "_nested",
        srcs = ["package_naming.bzl"],
        prefix = "/opt/agent/bin/agent/dist/views/css",
    )
    util.helper_target(
        pkg_files,
        name = name + "_relative",
        srcs = ["dd_pkg_deb.bzl"],
        prefix = "opt/agent/LICENSES",
    )
    util.helper_target(
        pkg_files,
        name = name + "_outside",
        srcs = ["dd_pkg_rpm.bzl"],
        prefix = "/etc/agent/conf.d",
    )
    util.helper_target(
        pkg_mkdirs,
        name = name + "_declared",
        dirs = ["/opt/agent/bin", "/opt/agent/embedded/lib"],
    )
    util.helper_target(
        pkg_mklink,
        name = name + "_link",
        link_name = "/opt/agent/sources/systemd/link",
        target = "/nowhere",
    )
    util.helper_target(
        _tree,
        name = name + "_tree",
    )
    util.helper_target(
        pkg_files,
        name = name + "_tree_files",
        srcs = [name + "_tree"],
        prefix = "/opt/agent/embedded/include",
        renames = {name + "_tree": "openssl"},
    )
    util.helper_target(
        pkg_filegroup,
        name = name + "_group",
        srcs = [name + "_nested", name + "_link"],
    )
    util.helper_target(
        dd_pkg_owned_dirs,
        name = name + "_subject",
        srcs = [
            name + "_group",
            name + "_relative",
            name + "_outside",
            name + "_declared",
            name + "_tree_files",
        ],
    )
    analysis_test(
        name = name,
        impl = _test_parent_dirs_impl,
        target = name + "_subject",
        config_settings = {str(Label("//:install_dir")): "/opt/agent"},
    )

def _test_parent_dirs_impl(env, target):
    env.expect.that_collection(target[PackageDirsInfo].dirs).contains_exactly([
        "/opt/agent",
        "/opt/agent/LICENSES",
        "/opt/agent/bin/agent",
        "/opt/agent/bin/agent/dist",
        "/opt/agent/bin/agent/dist/views",
        "/opt/agent/bin/agent/dist/views/css",
        "/opt/agent/embedded",
        "/opt/agent/embedded/include",
        "/opt/agent/embedded/include/openssl",
        "/opt/agent/sources",
        "/opt/agent/sources/systemd",
    ])

def dd_pkg_owned_dirs_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_parent_dirs,
        ],
    )
