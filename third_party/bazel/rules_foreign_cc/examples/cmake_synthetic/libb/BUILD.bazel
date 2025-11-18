load("@rules_cc//cc:defs.bzl", "cc_library")

filegroup(
    name = "b_srcs",
    srcs = glob(["**"]),
    visibility = ["//visibility:public"],
)

cc_library(
    name = "lib_b_bazel",
    srcs = ["src/libb.cpp"],
    hdrs = ["src/libb.h"],
    includes = ["src"],
    visibility = ["//visibility:public"],
    deps = ["//cmake_synthetic/liba:lib_a_bazel"],
)
