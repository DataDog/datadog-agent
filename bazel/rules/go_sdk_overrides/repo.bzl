"""Repository rule for patching Go SDK packages into a local repo."""

load("@go_host_compatible_sdk_label//:defs.bzl", "HOST_COMPATIBLE_SDK")
load("@rules_python//python/private:repo_utils.bzl", "repo_utils")  # buildifier: disable=bzl-visibility
load("//bazel/rules/go_sdk_overrides:defs.bzl", "OVERRIDES", "PATCHES")

def _impl(rctx):
    rctx.file("BUILD.bazel", "exports_files({})".format([build.package for build in rctx.attr.overrides.values()]))
    sdk = rctx.path(rctx.attr._go_sdk).dirname
    for src_dir, build in rctx.attr.overrides.items():
        rctx.file("{}/{}".format(build.package, build.name), rctx.read(build))
        for f in sdk.get_child(src_dir).readdir():
            if f.basename.endswith(".go") and not f.basename.endswith("_test.go"):
                rctx.file("{}/{}".format(build.package, f.basename), rctx.read(f))
    go = sdk.get_child("bin/go{}".format(".exe" if repo_utils.get_platforms_os_name(rctx) == "windows" else ""))
    for p in rctx.attr.patches:
        if p.name.endswith(".gopatch"):
            repo_utils.execute_checked(
                rctx,
                op = "gopatch {}".format(p),
                arguments = [go, "-C", rctx.path(rctx.attr._gopatch).dirname, "run", ".", "-p", p, rctx.path(".")],
                environment = {"GOPATH": None, "GOROOT": None, "GOWORK": "off"},
            )
        else:
            rctx.patch(p, strip = 1)

go_sdk_overrides = repository_rule(
    implementation = _impl,
    attrs = {
        "_go_sdk": attr.label(default = HOST_COMPATIBLE_SDK),
        "_gopatch": attr.label(default = "@com_github_uber_go_gopatch//:gopatch"),
        "overrides": attr.string_keyed_label_dict(default = OVERRIDES),
        "patches": attr.label_list(default = PATCHES),
    },
)
