"""Repository rule that materializes a hermetic MinGW-w64 toolchain from a
pinned WinLibs distribution.

Pins URL, SHA256 and GCC version together in MODULE.bazel so bumping the zip
and the include-dir layout it implies stays a single atomic change.
"""

def _winlibs_mingw_repository_impl(ctx):
    ctx.download_and_extract(
        url = ctx.attr.url,
        sha256 = ctx.attr.sha256,
        stripPrefix = ctx.attr.strip_prefix,
    )
    ctx.template(
        "BUILD.bazel",
        ctx.attr._build_file_template,
        substitutions = {
            "%GCC_VERSION%": ctx.attr.gcc_version,
        },
        executable = False,
    )

winlibs_mingw_repository = repository_rule(
    implementation = _winlibs_mingw_repository_impl,
    doc = "Downloads a pinned WinLibs MinGW-w64 zip and exposes it as a hermetic cc_toolchain.",
    attrs = {
        "url": attr.string(
            mandatory = True,
            doc = "Direct URL to the WinLibs .zip release asset.",
        ),
        "sha256": attr.string(
            mandatory = True,
            doc = "SHA256 of the zip, pinned for supply-chain integrity.",
        ),
        "strip_prefix": attr.string(
            default = "mingw64",
            doc = "Top-level directory stripped from the zip (WinLibs ships everything under mingw64/).",
        ),
        "gcc_version": attr.string(
            mandatory = True,
            doc = "GCC version baked into the WinLibs zip; must match the URL or builtin include dirs will be wrong.",
        ),
        "_build_file_template": attr.label(
            default = "//bazel/toolchains/mingw:winlibs.BUILD.bazel",
            allow_single_file = True,
        ),
    },
)
