load("@rules_cc//cc:defs.bzl", "cc_test")
load("@rules_foreign_cc//foreign_cc:defs.bzl", "make")

make(
    name = "make_lib",
    build_data = [
        "//make_simple/code:cxx_wrapper.sh",
        "README.md",
    ],
    copts = [
        "-DREQUIRED_DEFINE",
    ],
    env = {
        "CXX_WRAPPER": "$(execpath //make_simple/code:cxx_wrapper.sh)",
        "README_DIR": "$$(dirname $(execpath README.md))",
    },
    lib_source = "//make_simple/code:srcs",
    out_data_dirs = ["share"],
    out_static_libs = ["liba.a"],
)

cc_test(
    name = "test_lib",
    srcs = [
        "test_libb.cpp",
    ],
    copts = [
        "-std=c++11",
    ],
    tags = ["windows"],
    deps = [
        ":make_lib",
    ],
)
