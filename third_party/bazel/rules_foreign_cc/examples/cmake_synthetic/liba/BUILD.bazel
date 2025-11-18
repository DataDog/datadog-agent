load("@rules_cc//cc:defs.bzl", "cc_library")

filegroup(
    name = "a_srcs",
    srcs = glob(["**"]),
    visibility = ["//visibility:public"],
)

cc_library(
    name = "lib_a_bazel",
    srcs = ["src/liba.cpp"],
    hdrs = ["src/liba.h"],
    includes = ["src"],
    visibility = ["//visibility:public"],
)
