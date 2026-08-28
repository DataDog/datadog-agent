"""Macros building the OTel TLS testers, one per access model."""

load("@rules_cc//cc:cc_binary.bzl", "cc_binary")
load("@rules_cc//cc:cc_import.bzl", "cc_import")

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
DSO_FLAVORS = {
    "": _TLS_DIALECT,
    "_gnu": _TLS_ALT_DIALECT,
    "_ie": ["-ftls-model=initial-exec"],
    "_ld": ["-DOTEL_TLS_HIDDEN", "-ftls-model=local-dynamic"] + _TLS_ALT_DIALECT,
}

# The resolver only supports these two architectures, and the testers are
# Linux-only; target_compatible_with is a conjunction, so the CPU alternatives
# have to come through a select.
SUPPORTED_PLATFORMS = ["@platforms//os:linux"] + select({
    "@platforms//cpu:aarch64": [],
    "@platforms//cpu:x86_64": [],
    "//conditions:default": ["@platforms//:incompatible"],
})

def otel_tls_dso(suffix, copts):
    """Emits one shared object per access model plus the binary linking it at startup.

    Args:
        suffix: flavor suffix appended to the artifact names, "" for the default dialect.
        copts: the compiler flags selecting this flavor's access model.
    """
    so = "libotel_tls_glibc%s.so" % suffix

    cc_binary(
        name = so,
        srcs = [
            "otel_tls_lib.c",
            ":otel_tls_headers",
        ],
        copts = ["-fPIC"] + copts,
        linkopts = [
            "-Wl,-soname,%s" % so,
            "-pthread",
        ],
        linkshared = True,
        target_compatible_with = SUPPORTED_PLATFORMS,
        visibility = ["//visibility:public"],
    )

    cc_import(
        name = "otel_tls_glibc%s_import" % suffix,
        shared_library = so,
    )

    cc_binary(
        name = "otel_tls_linked_glibc%s" % suffix,
        srcs = [
            "otel_tls_driver.c",
            ":otel_tls_headers",
        ],
        # $ORIGIN, not the build directory: the test harness copies every
        # artifact into one directory of its own before running them.
        linkopts = ["-Wl,-rpath,$$ORIGIN"],
        target_compatible_with = SUPPORTED_PLATFORMS,
        visibility = ["//visibility:public"],
        deps = ["otel_tls_glibc%s_import" % suffix],
    )
