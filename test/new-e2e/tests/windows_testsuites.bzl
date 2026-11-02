"""Lays out Windows builds of tests for e2e suites to run them remotely, as `go test -c` would have built them."""

load("@bazel_skylib//lib:paths.bzl", "paths")
load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@rules_pkg//pkg:providers.bzl", "PackageFilesInfo")

def _files_impl(ctx):
    dest_src_map = {}
    for test, dest in ctx.attr.files.items():
        dest_src_map[dest] = test[DefaultInfo].files_to_run.executable
        testdata = "{}/testdata/".format(test.label.package)
        for f in test[DefaultInfo].default_runfiles.files.to_list():  # depset.to_list() is fine as the rule is terminal
            if f.short_path.startswith(testdata):
                dest_src_map[paths.join(paths.dirname(dest), paths.relativize(f.short_path, test.label.package))] = f
    return [
        DefaultInfo(files = depset(dest_src_map.values())),
        PackageFilesInfo(attributes = {"mode": "0755"}, dest_src_map = dest_src_map),
    ]

_files = rule(
    implementation = _files_impl,
    attrs = {
        "files": attr.label_keyed_string_dict(
            cfg = transition(
                implementation = lambda *_: {"//command_line_option:platforms": "//bazel/platforms:windows_x86_64"},
                inputs = [],
                outputs = ["//command_line_option:platforms"],
            ),
            mandatory = True,
        ),
    },
)

def _impl(name, visibility, files, **kwargs):
    files_name = "{}_files".format(name)
    _files(name = files_name, files = files, **kwargs)
    pkg_install(name = name, srcs = [files_name], visibility = visibility, **kwargs)

install_windows_testsuites = macro(
    implementation = _impl,
    inherit_attrs = _files,
    attrs = {
        "target_compatible_with": attr.label_list(default = ["@platforms//cpu:x86_64", "@platforms//os:linux"]),
        "testonly": attr.bool(default = True, configurable = False),
    },
)
