"""Repository rule that materializes a hermetic MSYS2 base environment from a
pinned MSYS2 distribution archive, plus a curated set of pacman packages
overlaid on top.

Pins URL, SHA256 and release date together in MODULE.bazel so bumping the
archive stays a single atomic change.

Layer 1 — `base` archive (msys2-base-*.tar.zst):
  bash, coreutils, sed, gawk, grep, find, tar, gzip, xz, msys-2.0.dll runtime.

Layer 2 — `overlay_packages` (.pkg.tar.zst from repo.msys2.org/msys/x86_64/):
  Each pacman package is a tarball that ships its files under the same
  `usr/<bin|lib|share|include>/` layout as the base, so extracting at the
  same root merges cleanly. We use this to provide `make`, `perl`, `pkgconf`,
  autotools, etc. without falling back to rules_foreign_cc's slow source
  bootstrap (which on Windows ends up trying to compile glib for pkg-config).

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

    # Starlark dicts preserve insertion order, so the extract sequence is
    # deterministic and matches the MODULE.bazel declaration order — useful
    # when two packages own the same file (rare for MSYS2; last write wins).
    for pkg_name, spec in ctx.attr.overlay_packages.items():
        if len(spec) != 2:
            fail("overlay_packages[%r] must be [url, sha256], got %r" % (pkg_name, spec))
        ctx.download_and_extract(
            url = spec[0],
            sha256 = spec[1],
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
        "overlay_packages": attr.string_list_dict(
            doc = "Pacman packages overlaid on the base tree, as name -> [url, sha256].",
        ),
        "_build_file_template": attr.label(
            default = "//bazel/toolchains/msys2:msys2.BUILD.bazel",
            allow_single_file = True,
        ),
    },
)
