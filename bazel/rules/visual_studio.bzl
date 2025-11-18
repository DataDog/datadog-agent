"""Wrapping Visual Studio and MSBuild to let Bazel track it.

We're very probably never going to get nothing close to hermetic for these tools (less so considering
sandboxing limitations in Windows); this provides a best-effort attempt at at least making Bazel
manage and track these tools.
"""

def _visual_studio_impl(ctx):
    # vswhere is a tool that lets us inspect existing Visual Studio installations
    ctx.report_progress("Download vswhere.exe")
    ctx.download(
        "https://github.com/microsoft/vswhere/releases/download/3.1.7/vswhere.exe",
        output="vswhere.exe",
        sha256="c54f3b7c9164ea9a0db8641e81ecdda80c2664ef5a47c4191406f848cc07c662",
        executable=True,
    )

    ctx.report_progress("Downloading VS installer")
    ctx.download(
        ctx.attr.url,
        output='vs_buildtools.exe',
        sha256=ctx.attr.sha256,
        executable=True,
    )

    installer_path = str(ctx.path("vs_buildtools.exe")).replace("/", "\\")
    install_folder = "VisualStudio"
    install_path = str(ctx.path(install_folder)).replace("/", "\\")

    # Check for an existing installation
    instance_id = _get_vs_instance(ctx, install_path)

    if instance_id:
        # We need to clean up the existing installation as otherwise the installer won't do anything
        print("Found existing VS instance id: %s. This will be uninstalled before proceeding." % instance_id)
        ctx.report_progress("Cleaning up existing installation")

        result = ctx.execute(
            _vs_installer_command(
                installer_path, ["uninstall", "--installPath", install_path]
            )
        )
        if result.return_code:
            fail("Failed to remove existing VS: %s" % result.stderr)

    vs_packages = [
        "Microsoft.Component.MSBuild",
        "Microsoft.VisualStudio.Workload.NativeDesktop",
        "Microsoft.VisualStudio.Workload.VCTools",
        "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
    ]
    vs_packages_args = []
    for pkg in vs_packages:
        vs_packages_args += ["--add", pkg]

    ctx.report_progress("Installing Visual Studio")

    result = ctx.execute(
        _vs_installer_command(
            installer_path, ["--installPath", install_path] + vs_packages_args
        )
    )

    if result.return_code:
        fail("Failed to install VS: %s" % result.stderr)

    if not ctx.path("VisualStudio").is_dir:
        fail("Failed to install VS: expected VisualStudio folder was not created.")

    build_file_content = """
exports_files(["VisualStudio/MSBuild/Current/Bin/msbuild.exe"])

alias(
    name="msbuild",
    actual="VisualStudio/MSBuild/Current/Bin/msbuild.exe",
    visibility = ["//visibility:public"],
)
"""
    ctx.file("BUILD.bazel", build_file_content)

    # This makes changes to the system so it's not really reproducible.
    # The rule itself will take care of avoiding reinstalling if we think we're already set
    return ctx.repo_metadata(reproducible=False)


def windows_path(path):
    return str(path).replace("/", "\\")


def _get_vs_instance(ctx, install_path):
    result = ctx.execute([
        windows_path(ctx.path("vswhere.exe")),
        "-nologo",
        "-nocolor",
        "-property",
        "instanceId",
        "-path",
        install_path,
    ])
    return result.stdout


def _vs_installer_command(installer_path, args):
    """Helper to construct a list of command args for ctx.execute for the VS installer,
    with the required wrapping.

    Trying to execute the installer directly doesn't work and just hangs.
    """
    arg_list = ["--wait", "--quiet", "--norestart", "--nocache"] + args

    return [
        "powershell.exe",
        "-Command", "Start-Process",
        "-FilePath", installer_path,
        "-NoNewWindow",
        "-Wait",
        "-ArgumentList", ",".join(arg_list),
    ]


visual_studio = repository_rule(
    implementation = _visual_studio_impl,
    attrs = {
        "url": attr.string(
            mandatory=True,
            doc="URL of Visual Studio installer",
        ),
        "sha256": attr.string(
            mandatory=True,
            doc="Expected SHA-256 of VS installer",
        ),
        "extra_packages": attr.string_list(
            doc="Additional components and workloads to install on top of the default ones",
        ),
    },
    configure = True,
    doc =
    """Installs Visual Studio and MSBuild.

    Note that this will never be truly hermetic. In particular, Visual Studio instances,
    even if to some extent isolated, also write to a folder they own under
    %LOCALAPPDATA%\\Microsoft\\VisualStudio.
    """
)
