"""Mirrors the pinned Go SDK, patching it with the same CI Visibility hooks Orchestrion would otherwise weave in.
Everything else symlinks back to the real SDK.
"""

load("@bazel_skylib//lib:paths.bzl", "paths")
load("@rules_python//python/private:repo_utils.bzl", "repo_utils")  # buildifier: disable=bzl-visibility

_PATCHED = set([
    "src/testing/benchmark.go",
    "src/testing/testing.go",
])

def _impl(rctx):
    sdk = rctx.path(rctx.attr._go_sdk).dirname
    rctx.watch(sdk.get_child("VERSION"))  # invalidate on SDK bump, since symlink targets may not be watched

    # 1. apply patches to copied files
    for path in _PATCHED:
        rctx.file(path, rctx.read(sdk.get_child(path)))
    rctx.patch(rctx.attr.patch)

    # 2. add symlinks to paths sibling to patched files
    for src_path in sdk.get_child("src").readdir():
        src_name = paths.join("src", src_path.basename)
        if src_path.basename == "testing":
            for testing_path in src_path.readdir():
                testing_name = paths.join(src_name, testing_path.basename)
                if testing_name not in _PATCHED:
                    rctx.symlink(testing_path, testing_name)
        else:
            rctx.symlink(src_path, src_name)

    # 3. add symlinks to paths foreign to patched files
    for name in ["ROOT", "VERSION", "bin", "go.env", "pkg"]:
        rctx.symlink(sdk.get_child(name), name)

    rctx.file(
        "BUILD.bazel",
        """load("@rules_go//go:def.bzl", "go_sdk", "go_toolchain")

# https://github.com/bazelbuild/rules_go/blob/v0.63.0/go/private/rules/sdk.bzl#L50
go_sdk(
    name = "go_civisibility_sdk",
    srcs = glob(["src/**/*"]),
    go = ":bin/{go_name}",
    goarch = "amd64",
    goos = "linux",
    headers = glob(["pkg/include/*.h"]),
    root_file = "ROOT",
    tools = glob(["pkg/tool/**", "bin/gofmt*"]),
)

# https://github.com/bazelbuild/rules_go/blob/v0.63.0/go/private/go_toolchain.bzl#L57
go_toolchain(
    name = "go_civisibility_linux_amd64-impl",
    builder = "@go_work_sdk//:builder_reset",
    goarch = "amd64",
    goos = "linux",
    pack = "@go_work_sdk//:pack_reset",
    sdk = ":go_civisibility_sdk",
    tags = ["manual"],
)

# https://github.com/bazelbuild/rules_go/blob/v0.63.0/go/private/go_toolchain.bzl#L218
config_setting(
    name = "match_sdk_name",
    flag_values = {{"@rules_go//go/toolchain:sdk_name": "go_civisibility_sdk"}},
)

# https://github.com/bazelbuild/rules_go/blob/v0.63.0/go/private/go_toolchain.bzl#L254
toolchain(
    name = "go_civisibility_linux_amd64",
    exec_compatible_with = [
        "@rules_go//go/toolchain:linux",
        "@rules_go//go/toolchain:amd64",
    ],
    target_compatible_with = [
        "@platforms//os:linux",
        "@platforms//cpu:x86_64",
    ],
    target_settings = [":match_sdk_name"],
    toolchain = ":go_civisibility_linux_amd64-impl",
    toolchain_type = "@rules_go//go:toolchain",
)
""".format(go_name = "go.exe" if repo_utils.get_platforms_os_name(rctx) == "windows" else "go"),
    )

go_civisibility_sdk = repository_rule(
    implementation = _impl,
    attrs = {
        "_go_sdk": attr.label(default = "@go_work_sdk//:ROOT"),
        "patch": attr.label(default = "//bazel/rules/go_civisibility_sdk:go_civisibility_sdk.patch"),
    },
)
