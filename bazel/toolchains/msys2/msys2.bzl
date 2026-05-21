"""Repository rule that materializes a hermetic MSYS2 base environment from a
pinned MSYS2 distribution archive.

Pins URL, SHA256 and release date together in MODULE.bazel so bumping the
archive stays a single atomic change.

The MSYS2 `base` package and its transitive dependencies are extracted as-is:
bash, coreutils, sed, gawk, grep, find, tar, gzip, xz, plus the msys-2.0.dll
runtime. No `make`/`perl`/autotools yet — those are separate pacman packages
and will be overlaid in a follow-up if rules_foreign_cc workloads require them.

Skips the upstream first-run post-install (which generates /etc/passwd,
/etc/group and pacman gpg keyrings) because:
  * passwd/group are auto-synthesised by the Cygwin/MSYS2 runtime on demand
    since cygwin 1.7.34 (2015);
  * gpg keyrings are only consumed by pacman, which we never run hermetically.

`sh_toolchain.path` is a string attribute documented as "Absolute path to the
shell interpreter". We compute the absolute path to the extracted bash.exe at
fetch time and bake it into the generated BUILD.bazel via template
substitution — the standard pattern for hermetic-but-absolute-path toolchains
on Windows.
"""

def _msys2_base_repository_impl(ctx):
    ctx.download_and_extract(
        url = ctx.attr.url,
        sha256 = ctx.attr.sha256,
        stripPrefix = ctx.attr.strip_prefix,
    )

    # Bazel passes this path verbatim to its action executor; forward slashes
    # work for both cmd.exe and bash on Windows, native backslashes do not
    # round-trip through Starlark string handling cleanly.
    bash_abs_path = str(ctx.path("usr/bin/bash.exe")).replace("\\", "/")

    ctx.template(
        "BUILD.bazel",
        ctx.attr._build_file_template,
        substitutions = {
            "%VERSION%": ctx.attr.version,
            "%BASH_ABSOLUTE_PATH%": bash_abs_path,
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
        "_build_file_template": attr.label(
            default = "//bazel/toolchains/msys2:msys2.BUILD.bazel",
            allow_single_file = True,
        ),
    },
)
