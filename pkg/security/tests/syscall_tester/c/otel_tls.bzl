"""Macros building the OTel thread context testers: the native ones, one per TLS
access model, and the Node.js one, which publishes through a discovery
thread-local instead."""

load("@rules_cc//cc:cc_binary.bzl", "cc_binary")
load("@rules_cc//cc:cc_library.bzl", "cc_library")
load("@rules_cc//cc:cc_shared_library.bzl", "cc_shared_library")

# -mtls-dialect spells the two general-dynamic dialects differently per
# architecture; the rest of the flags are the same everywhere.
_TLS_DIALECT = select({
    "@platforms//cpu:aarch64": ["-mtls-dialect=desc"],
    "@platforms//cpu:x86_64": ["-mtls-dialect=gnu2"],
})

_TLS_ALT_DIALECT = select({
    "@platforms//cpu:aarch64": ["-mtls-dialect=trad"],
    "@platforms//cpu:x86_64": ["-mtls-dialect=gnu"],
})

# The access models, mirroring the matrix the upstream profiler builds for its
# own integration tests (open-telemetry/opentelemetry-ebpf-profiler#1229).
# local-dynamic only appears when the TLS symbol is hidden, hence
# OTEL_TLS_HIDDEN.
_DSO_FLAVORS = {
    "": _TLS_DIALECT,
    "_gnu": _TLS_ALT_DIALECT,
    "_ie": ["-ftls-model=initial-exec"],
    "_ld": ["-DOTEL_TLS_HIDDEN", "-ftls-model=local-dynamic"] + _TLS_ALT_DIALECT,
}

def otel_tls_dsos(name):
    """Emits one shared object per access model plus the binary linking it at startup.

    Args:
        name: name of the filegroup listing every artifact this emits.
    """
    artifacts = []
    for suffix, copts in _DSO_FLAVORS.items():
        lib = "otel_tls_glibc%s" % suffix

        cc_library(
            name = "%s_srcs" % lib,
            srcs = [
                "otel_tls_lib.c",
                ":otel_tls_headers",
            ],
            copts = ["-fPIC", "-pthread"] + copts,
            linkopts = ["-pthread"],
            target_compatible_with = ["@platforms//os:linux"],
        )

        # cc_shared_library derives the .so filename from `name`; the binary
        # below links against it by that literal filename via dynamic_deps.
        cc_shared_library(
            name = lib,
            deps = [":%s_srcs" % lib],
            target_compatible_with = ["@platforms//os:linux"],
            visibility = ["//visibility:public"],
        )

        linked = "otel_tls_linked_glibc%s" % suffix
        cc_binary(
            name = linked,
            srcs = [
                "otel_tls_driver.c",
                ":otel_tls_headers",
            ],
            dynamic_deps = [":%s" % lib],
            # $ORIGIN, not the build directory: the test harness copies every
            # artifact into one directory of its own before running them.
            linkopts = ["-Wl,-rpath,$$ORIGIN"],
            target_compatible_with = ["@platforms//os:linux"],
            visibility = ["//visibility:public"],
        )

        artifacts += [":%s" % lib, ":%s" % linked]

    native.filegroup(
        name = name,
        srcs = artifacts,
        visibility = ["//visibility:public"],
    )

def otel_nodejs_dso(name):
    """Emits the Node.js tester's shared object plus the binary linking it at startup.

    Only the TLSDESC/general-dynamic dialect, and only out of a shared object: that
    is how a Node addon publishes the discovery thread-local, and the native
    testers above already cover the other access models.

    Args:
        name: name of the filegroup listing every artifact this emits.
    """
    lib = "otel_nodejs_glibc"

    cc_library(
        name = "%s_srcs" % lib,
        srcs = [
            "otel_nodejs_lib.c",
            ":otel_nodejs_headers",
        ],
        copts = ["-fPIC", "-pthread"] + _TLS_DIALECT,
        linkopts = ["-pthread"],
        target_compatible_with = ["@platforms//os:linux"],
    )

    cc_shared_library(
        name = lib,
        deps = [":%s_srcs" % lib],
        target_compatible_with = ["@platforms//os:linux"],
        visibility = ["//visibility:public"],
    )

    linked = "otel_nodejs_linked_glibc"
    cc_binary(
        name = linked,
        srcs = [
            "otel_nodejs_driver.c",
            ":otel_nodejs_headers",
        ],
        dynamic_deps = [":%s" % lib],
        # $ORIGIN, not the build directory: the test harness copies every
        # artifact into one directory of its own before running them.
        linkopts = ["-Wl,-rpath,$$ORIGIN"],
        target_compatible_with = ["@platforms//os:linux"],
        visibility = ["//visibility:public"],
    )

    native.filegroup(
        name = name,
        srcs = [":%s" % lib, ":%s" % linked],
        visibility = ["//visibility:public"],
    )
