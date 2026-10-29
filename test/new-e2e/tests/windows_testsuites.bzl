"""Lays out Windows builds of tests for e2e suites to run them remotely, as `go test -c` would have built them."""

load("@bazel_skylib//lib:paths.bzl", "paths")

def _files_impl(ctx):
    artifacts = paths.join(ctx.label.package, "artifacts")
    symlinks = {}
    for test, dest in ctx.attr.files.items():
        symlinks[paths.join(artifacts, dest)] = test[DefaultInfo].files_to_run.executable
        testdata = "{}/testdata/".format(test.label.package)
        for f in test[DefaultInfo].default_runfiles.files.to_list():  # depset.to_list() is fine as the rule is terminal
            if f.short_path.startswith(testdata):
                symlinks[paths.join(artifacts, paths.dirname(dest), paths.relativize(f.short_path, test.label.package))] = f
    return [DefaultInfo(runfiles = ctx.runfiles(symlinks = symlinks))]

_files = rule(
    implementation = _files_impl,
    attrs = {
        "files": attr.label_keyed_string_dict(
            cfg = transition(
                implementation = lambda *_: {
                    "//command_line_option:platforms": "//bazel/platforms:windows_x86_64",
                    "@rules_go//go/toolchain:sdk_name": "",  # go_civisibility_sdk is for the job, not the remote VM
                },
                inputs = [],
                outputs = ["//command_line_option:platforms", "@rules_go//go/toolchain:sdk_name"],
            ),
            mandatory = True,
        ),
    },
)

def _impl(name, visibility, **kwargs):
    _files(name = name, visibility = visibility, **kwargs)

windows_testsuites = macro(
    implementation = _impl,
    inherit_attrs = _files,
    attrs = {
        "target_compatible_with": attr.label_list(default = ["@platforms//cpu:x86_64", "@platforms//os:linux"]),
        "testonly": attr.bool(default = True, configurable = False),
    },
)
