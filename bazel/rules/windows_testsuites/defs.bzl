"""Lays out Windows builds of tests for e2e suites to run them remotely, as `go test -c` would have built them."""

load("@bazel_skylib//lib:paths.bzl", "paths")
load("@rules_pkg//pkg:providers.bzl", "PackageFilesInfo")

def _windows_impl(_settings, _attr):
    return {"//command_line_option:platforms": str(Label("//bazel/platforms:windows_x86_64"))}

_windows = transition(
    implementation = _windows_impl,
    inputs = [],
    outputs = ["//command_line_option:platforms"],
)

def _windows_testsuites_impl(ctx):
    dest_src_map = {}
    for test, dest in ctx.attr.tests.items():
        dest_src_map[dest] = test[DefaultInfo].files_to_run.executable
        testdata = test.label.package + "/testdata/"
        for f in test[DefaultInfo].default_runfiles.files.to_list():
            if f.short_path.startswith(testdata):
                dest_src_map[paths.join(paths.dirname(dest), f.short_path.removeprefix(test.label.package + "/"))] = f
    return [
        DefaultInfo(files = depset(dest_src_map.values())),
        PackageFilesInfo(attributes = {"mode": "0755"}, dest_src_map = dest_src_map),
    ]

_windows_testsuites = rule(
    implementation = _windows_testsuites_impl,
    doc = "Maps Windows builds of tests, and their `testdata` next to them, to destinations for `pkg_install`.",
    attrs = {
        "tests": attr.label_keyed_string_dict(
            cfg = _windows,
            doc = "Tests to build for Windows, mapped to their destination",
            mandatory = True,
        ),
    },
)

def _windows_testsuites_macro_impl(name, visibility, **kwargs):
    _windows_testsuites(
        name = name,
        visibility = visibility,
        **kwargs
    )

windows_testsuites = macro(
    doc = "Maps Windows builds of tests, and their `testdata` next to them, to destinations for `pkg_install`.",
    inherit_attrs = _windows_testsuites,
    attrs = {
        # the only non-Windows hosts able to build Windows resources, see //bazel/rules:windows_message_compiler
        "target_compatible_with": attr.label_list(default = ["@platforms//cpu:x86_64", "@platforms//os:linux"]),
    },
    implementation = _windows_testsuites_macro_impl,
)
