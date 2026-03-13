def _impl(rctx):
    rctx.file("stringer.go", rctx.read(rctx.attr.stringer_orig))
    rctx.patch(rctx.attr.stringer_patch)
    rctx.file("BUILD.bazel", """load("@rules_go//go:def.bzl", "go_binary")

go_binary(
    name = "go_stringer",
    srcs = ["stringer.go"],
    visibility = ["//visibility:public"],
    deps = ["@org_golang_x_tools//go/packages"],
)
""")

go_stringer = repository_rule(
    implementation = _impl,
    attrs = {
        "stringer_orig": attr.label(default = "@org_golang_x_tools//cmd/stringer:stringer.go"),
        "stringer_patch": attr.label(default = "//bazel/rules/go_stringer:stringer.go.patch"),
    },
)
