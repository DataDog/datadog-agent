"""pkg_mac_pkg - build a macOS .pkg installer from rules_pkg-style file mappings.

Consumes the same `srcs` shape as `pkg_deb`/`pkg_rpm` (pkg_files/pkg_filegroup
targets) and calls the macOS `pkgbuild` tool to produce a single component
package. See ABLD-395 (~/mac_package.md has the design writeup).

v1 scope, deliberately: no `productbuild`/Distribution.xml composition -- a
single flat component package is directly installable via `installer(8)`,
Installer.app, or MDM. v1 also materializes `srcs` to a real directory on disk
before invoking `pkgbuild --root`, reusing rules_pkg's own `pkg_install`
installer (and its codesign-safe atomic file copy) rather than writing a
bespoke manifest interpreter.
"""

load("@rules_pkg//pkg:install.bzl", "pkg_install")

def _pkg_mac_pkg_impl(ctx):
    root_dir = ctx.actions.declare_directory(ctx.label.name + "_root")

    # `pkg_install` produces a `bazel run`-able installer whose CLI insists on
    # an absolute --destdir (or BUILD_WORKSPACE_DIRECTORY for resolving a
    # relative one), since it's normally invoked interactively. Inside a
    # Bazel action the process cwd is already the exec root, so a relative
    # destdir resolves correctly on disk -- we just need to satisfy that
    # check. Setting BUILD_WORKSPACE_DIRECTORY to "." (a literal we choose,
    # not an inherited env var) does that without needing our own installer.
    ctx.actions.run(
        mnemonic = "MacPkgRoot",
        progress_message = "Materializing pkg root for %s" % ctx.label,
        executable = ctx.attr.installer[DefaultInfo].files_to_run,
        arguments = ["--destdir", root_dir.path, "--wipe_destdir"],
        env = {"BUILD_WORKSPACE_DIRECTORY": "."},
        outputs = [root_dir],
    )

    output = ctx.outputs.out
    inputs = [root_dir, ctx.file._materialize_root_py]

    preinstall_path = ""
    if ctx.file.preinstall:
        preinstall_path = ctx.file.preinstall.path
        inputs.append(ctx.file.preinstall)

    postinstall_path = ""
    if ctx.file.postinstall:
        postinstall_path = ctx.file.postinstall.path
        inputs.append(ctx.file.postinstall)

    ctx.actions.run(
        mnemonic = "MacPkgBuild",
        progress_message = "Building macOS pkg %s" % ctx.label,
        executable = ctx.executable._pkgbuild_wrapper,
        arguments = [
            ctx.file._materialize_root_py.path,
            root_dir.path,
            ctx.attr.identifier,
            ctx.attr.version,
            ctx.attr.install_location,
            output.path,
            preinstall_path,
            postinstall_path,
            ctx.attr.signing_identity,
        ],
        inputs = inputs,
        outputs = [output],
    )

    return [
        DefaultInfo(files = depset([output])),
        OutputGroupInfo(pkg = depset([output])),
    ]

_pkg_mac_pkg = rule(
    implementation = _pkg_mac_pkg_impl,
    attrs = {
        "installer": attr.label(
            mandatory = True,
            executable = True,
            cfg = "target",
            providers = [DefaultInfo],
            doc = "pkg_install target used to materialize srcs onto disk.",
        ),
        "identifier": attr.string(mandatory = True, doc = "pkgbuild --identifier"),
        "version": attr.string(mandatory = True, doc = "pkgbuild --version"),
        "install_location": attr.string(default = "/", doc = "pkgbuild --install-location"),
        "preinstall": attr.label(allow_single_file = True, doc = "Script run as `preinstall`."),
        "postinstall": attr.label(allow_single_file = True, doc = "Script run as `postinstall`."),
        "signing_identity": attr.string(doc = "pkgbuild --sign identity name."),
        "out": attr.output(mandatory = True),
        "_pkgbuild_wrapper": attr.label(
            default = Label("//bazel/rules/macos/pkg:build_mac_pkg"),
            executable = True,
            cfg = "exec",
        ),
        "_materialize_root_py": attr.label(
            default = Label("//bazel/rules/macos/pkg:materialize_root.py"),
            allow_single_file = True,
        ),
    },
)

def pkg_mac_pkg(
        name,
        srcs,
        identifier,
        version,
        install_location = "/",
        preinstall = None,
        postinstall = None,
        signing_identity = "",
        out = None,
        target_compatible_with = None,
        **kwargs):
    """Builds a macOS .pkg installer from pkg_filegroup/pkg_files srcs.

    Args:
        name: rule name.
        srcs: pkg_filegroup framework mapping/grouping targets (same shape
            accepted by pkg_tar/pkg_install).
        identifier: the package identifier, e.g. "com.datadoghq.agent".
        version: the package version string.
        install_location: root install path baked into the package
            (pkgbuild --install-location). Defaults to "/".
        preinstall: optional label of a script to run as `preinstall`.
        postinstall: optional label of a script to run as `postinstall`.
        signing_identity: optional codesigning identity for `pkgbuild --sign`.
            Leave unset for unsigned builds; callers that need env-gated
            signing (e.g. Omnibus's SIGN_MAC) should select() this at the
            call site rather than reading the env from within the rule.
        out: output file name. Defaults to "{name}.pkg".
        target_compatible_with: defaults to macOS-only, since this rule
            shells out to the macOS `pkgbuild` binary.
        **kwargs: forwarded to the underlying targets (e.g. visibility).
    """
    pkg_install(
        name = name + "_installer",
        srcs = srcs,
        tags = ["manual"],
        visibility = ["//visibility:private"],
    )

    _pkg_mac_pkg(
        name = name,
        installer = ":" + name + "_installer",
        identifier = identifier,
        version = version,
        install_location = install_location,
        preinstall = preinstall,
        postinstall = postinstall,
        signing_identity = signing_identity,
        out = out or (name + ".pkg"),
        target_compatible_with = target_compatible_with or ["@platforms//os:macos"],
        **kwargs
    )
