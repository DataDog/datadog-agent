load("@rules_cc//cc:defs.bzl", "cc_binary")

cc_binary(
    name = "lib_b",
    srcs = [
        "src/lib_b.cpp",
        "src/lib_b.h",
    ],
    linkshared = True,
    visibility = ["//cmake_with_data:__pkg__"],
)
