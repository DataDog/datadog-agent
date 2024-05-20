load("@rules_foreign_cc//foreign_cc:defs.bzl", "configure_make")

filegroup(
    name = "srcs",
    srcs = glob(["**"]),
    visibility = ["//visibility:public"],
)

configure_make(
    name = "python3",
    lib_source = ":srcs",
    configure_options = [
        "--with-ensurepip=yes",
        "--enable-ipv6",
        "--with-universal-archs=intel",
        "--enable-shared",
        "--with-dbmliborder=",
        "--enable-optimizations",
        # Fixes an issue with __DATE__ being set to undefined `redacted`
        # https://github.com/bazelbuild/rules_foreign_cc/issues/239#issuecomment-478167267
        "CFLAGS='-Dredacted=\"redacted\"'",
    ],
    copts = [],
    deps = [
        "@zlib//:zlib",
    ],
    out_binaries = [
        "python3",
        "pip3",
        "python3.11",
    ],
    out_data_dirs = [
        "lib",
    ],
    visibility = ["//visibility:public"],
)
