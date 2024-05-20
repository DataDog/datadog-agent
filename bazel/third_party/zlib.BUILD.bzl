load("@rules_foreign_cc//foreign_cc:defs.bzl", "cmake")

filegroup(
    name = "srcs",
    srcs = glob(["**"]),
    visibility = ["//visibility:public"],
)

cmake(
    name = "zlib",
    env = {
        "CFLAGS": "-fPIC",
    },
    lib_source = ":srcs",
    out_shared_libs = select({
        "@platforms//os:linux": [
            "libz.so.1",
        ],
    }),
    visibility = ["//visibility:public"],
)
