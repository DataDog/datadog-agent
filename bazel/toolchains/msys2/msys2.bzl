"""Repository rule for a pinned MSYS2 base archive plus optional overlay packages.

On Windows, materializes the tree under MSYS2_INSTALL_ROOT (default
C:/tools/msys64) when bash is missing or MSYS2_FORCE_INSTALL=1.

TODO{agent-build}:
  - Require a managed sentinel version and auto-reinstall on pin bumps.
  - Register rules_foreign_cc toolchains against @msys2_base filegroups.
"""

_DEFAULT_INSTALL_ROOT = "C:/tools/msys64"

def _bash_path(install_root):
    if install_root.endswith("/"):
        install_root = install_root[:-1]
    return install_root + "/usr/bin/bash.exe"

def _install_msys2(ctx, install_root, force):
    bash = _bash_path(install_root)
    if not force and ctx.path(bash).exists:
        return

    install_script = ctx.path(ctx.attr._install_script)
    source = ctx.path(".")
    result = ctx.execute(
        [
            "powershell.exe",
            "-NoProfile",
            "-ExecutionPolicy",
            "Bypass",
            "-File",
            install_script,
            "-Source",
            source,
            "-InstallRoot",
            install_root,
        ] + (["-Force"] if force else []),
        quiet = False,
    )
    if result.return_code != 0:
        fail("MSYS2 install to %s failed:\n%s\n%s" % (
            install_root,
            result.stdout,
            result.stderr,
        ))

def _msys2_base_repository_impl(ctx):
    install_root = ctx.getenv("MSYS2_INSTALL_ROOT") or _DEFAULT_INSTALL_ROOT
    install_root = install_root.replace("\\", "/")

    if ctx.os.name != "windows":
        ctx.template(
            "BUILD.bazel",
            ctx.attr._build_file_template,
            substitutions = {
                "%VERSION%": ctx.attr.version,
                "%INSTALL_ROOT%": install_root,
            },
            executable = False,
        )
        return

    force = (ctx.getenv("MSYS2_FORCE_INSTALL") or "0") == "1"

    # Relaxed "keep existing MSYS2" policy lives in tools/bazel.bat only. Do not
    # short-circuit here: a cached repo rule skip prevents reinstall when bash is
    # removed or MSYS2_FORCE_INSTALL is set later.
    ctx.download_and_extract(
        url = ctx.attr.url,
        sha256 = ctx.attr.sha256,
        stripPrefix = ctx.attr.strip_prefix,
    )

    for pkg_name, spec in ctx.attr.overlay_packages.items():
        if len(spec) != 2:
            fail("overlay_packages[%r] must be [url, sha256], got %r" % (pkg_name, spec))
        ctx.download_and_extract(
            url = spec[0],
            sha256 = spec[1],
        )

    _install_msys2(ctx, install_root, force)

    ctx.template(
        "BUILD.bazel",
        ctx.attr._build_file_template,
        substitutions = {
            "%VERSION%": ctx.attr.version,
            "%INSTALL_ROOT%": install_root,
        },
        executable = False,
    )

msys2_base_repository = repository_rule(
    implementation = _msys2_base_repository_impl,
    doc = "Downloads a pinned MSYS2 base archive and, on Windows, installs it under MSYS2_INSTALL_ROOT.",
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
        "_install_script": attr.label(
            default = "//bazel/toolchains/msys2:install.ps1",
            allow_single_file = True,
        ),
    },
    environ = [
        "MSYS2_FORCE_INSTALL",
        "MSYS2_INSTALL_ROOT",
    ],
)
