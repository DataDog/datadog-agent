"""Repository rule for building a patched cmd/go binary."""

load("@go_host_compatible_sdk_label//:defs.bzl", "HOST_COMPATIBLE_SDK")
load("@rules_python//python/private:repo_utils.bzl", "repo_utils")  # buildifier: disable=bzl-visibility

_PATCHED = [
    "src/cmd/go/internal/modload/buildlist.go",
    "src/cmd/go/internal/modload/load.go",
]

def _impl(rctx):
    sdk = rctx.path(rctx.attr._go_sdk).dirname
    replace = {}
    for path in _PATCHED:
        src = sdk.get_child(path)
        rctx.file(path, rctx.read(src))
        replace[str(src)] = path
    rctx.file("overlay.json", json.encode({"Replace": replace}))
    rctx.patch(rctx.attr.patch)
    go = sdk.get_child("bin/go{}".format(".exe" if repo_utils.get_platforms_os_name(rctx) == "windows" else ""))
    out = "{}.exe".format(rctx.original_name)  # like native_binary, since .exe is needed on Windows, otherwise harmless
    repo_utils.execute_checked(
        rctx,
        arguments = [go, "build", "-overlay", "overlay.json", "-o", out, "cmd/go"],
        environment = {"GOPATH": None, "GOROOT": str(sdk), "GOWORK": "off"},
        op = "build {}".format(out),
    )
    rctx.file(
        "BUILD.bazel",
        'alias(name = "{}", actual = "{}", visibility = ["//visibility:public"])'.format(rctx.original_name, out),
    )

go_fast = repository_rule(
    implementation = _impl,
    attrs = {
        "_go_sdk": attr.label(default = HOST_COMPATIBLE_SDK),
        "patch": attr.label(default = "//bazel/rules/go_fast:avoid-redundant-work-in-go-work-sync.patch"),
    },
)
