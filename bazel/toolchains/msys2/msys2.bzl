"""Repository rule for a pinned MSYS2 base archive plus optional overlay packages.

On Windows (native Bazel only — not Git Bash/WSL), extracts the archive into
@msys2_base and copies the tree to MSYS2_INSTALL_ROOT (default C:/tools/msys64).

Relaxed policy: skip the host install when bash already exists unless
MSYS2_FORCE_INSTALL=1. tools/bazel.bat only triggers fetch; install happens here.

TODO{agent-build}:
  - Require a managed sentinel version and auto-reinstall on pin bumps.
  - Register rules_foreign_cc toolchains against @msys2_base filegroups.
"""

_DEFAULT_INSTALL_ROOT = "C:/tools/msys64"

def _bash_path(install_root):
    if install_root.endswith("/"):
        install_root = install_root[:-1]
    return install_root + "/usr/bin/bash.exe"

def _to_windows_path(p):
    return p.replace("/", "\\")

def _source_root(ctx):
    if ctx.path("usr/bin/bash.exe").exists:
        return "."
    if ctx.path("msys64/usr/bin/bash.exe").exists:
        return "msys64"
    fail(
        "MSYS2 archive extracted but usr/bin/bash.exe is missing " +
        "(ctx.os.name=%r). Check strip_prefix and download logs." % ctx.os.name,
    )

def _install_msys2(ctx, install_root, force):
    bash = _bash_path(install_root)
    if not force and ctx.path(bash).exists:
        return

    install_script = str(ctx.path(ctx.attr._install_script))
    source = str(ctx.path(_source_root(ctx)))
    install_root_win = _to_windows_path(install_root)

    args = [
        "powershell.exe",
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        install_script,
        "-Source",
        source,
        "-InstallRoot",
        install_root_win,
    ]
    if force:
        args.append("-Force")

    result = ctx.execute(args, quiet = False)
    if result.return_code != 0:
        fail(
            "MSYS2 install to %s failed (exit %d).\nstdout:\n%s\nstderr:\n%s" % (
                install_root,
                result.return_code,
                result.stdout,
                result.stderr,
            ),
        )

    if not ctx.path(bash).exists:
        fail(
            "MSYS2 install reported success but bash is still missing at %s. " % bash +
            "Check write access to %s (admin may be required)." % install_root,
        )

def _msys2_base_repository_impl(ctx):
    install_root = ctx.getenv("MSYS2_INSTALL_ROOT") or _DEFAULT_INSTALL_ROOT
    install_root = install_root.replace("\\", "/")

    # repository_os.name is the lowercased Java os.name, e.g. "windows 11".
    if not ctx.os.name.startswith("windows"):
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
    bash = _bash_path(install_root)
    need_archive = force or not ctx.path(bash).exists

    if need_archive:
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
    doc = "Downloads MSYS2 into @msys2_base and installs it under MSYS2_INSTALL_ROOT on Windows.",
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
    local = True,
)
