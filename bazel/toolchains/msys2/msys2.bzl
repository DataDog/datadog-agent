"""Repository rule that materializes a hermetic MSYS2 environment from a
pinned MSYS2 distribution archive, plus optional overlay pacman packages.

Pins URL, SHA256 and release date together in MODULE.bazel so bumping the
archive stays a single atomic change.

`sh_toolchain.path` is a string attribute documented as "Absolute path to the
shell interpreter", but the rule only forwards the string to Bazel. Keep it
relative to the generated repository so action keys do not depend on the active
Bazel output_base.
"""

def _msys2_base_repository_impl(ctx):
    ctx.download_and_extract(
        url = ctx.attr.url,
        sha256 = ctx.attr.sha256,
        stripPrefix = ctx.attr.strip_prefix,
    )

    # Starlark dicts preserve insertion order, so extraction order remains
    # deterministic and follows MODULE.bazel declaration order.
    for pkg_name, spec in ctx.attr.overlay_packages.items():
        if len(spec) != 2:
            fail("overlay_packages[%r] must be [url, sha256], got %r" % (pkg_name, spec))
        ctx.download_and_extract(
            url = spec[0],
            sha256 = spec[1],
        )

    ctx.template(
        "BUILD.bazel",
        ctx.attr._build_file_template,
        substitutions = {
            "%VERSION%": ctx.attr.version,
        },
        executable = False,
    )

msys2_base_repository = repository_rule(
    implementation = _msys2_base_repository_impl,
    doc = "Downloads a pinned MSYS2 base archive and exposes its bash as a hermetic sh_toolchain.",
    attrs = {
        "url": attr.string(
            mandatory = True,
            doc = "Direct URL to the msys2-base-x86_64-*.tar.zst release asset.",
        ),
        "sha256": attr.string(
            mandatory = True,
            doc = "SHA256 of the archive, pinned for supply-chain integrity.",
        ),
        "strip_prefix": attr.string(
            default = "msys64",
            doc = "Top-level directory stripped from the archive (MSYS2 ships everything under msys64/).",
        ),
        "version": attr.string(
            mandatory = True,
            doc = "MSYS2 release date (e.g. 20260322); must match the URL.",
        ),
        "overlay_packages": attr.string_list_dict(
            doc = "Pacman packages overlaid on the base tree, as name -> [url, sha256].",
        ),
        "_build_file_template": attr.label(
            default = "//bazel/toolchains/msys2:msys2.BUILD.bazel",
            allow_single_file = True,
        ),
    },
)
