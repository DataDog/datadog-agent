load("@bazel_skylib//rules:build_test.bzl", "build_test")

exports_files(
    [
        "BUILD.glib.bazel",
    ],
    visibility = ["//visibility:public"],
)

build_test(
    name = "glib_build_test",
    targets = [
        "@glib//:glib",
    ],
    visibility = ["//:__pkg__"],
)
