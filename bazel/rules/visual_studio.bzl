"""Wrapping Visual Studio and MSBuild to let Bazel track it.
"""

def _visual_studio_impl(ctx):
    # vswhere is a tool that lets us inspect existing Visual Studio installations
    ctx.report_progress("Download vswhere.exe")
    ctx.download(
        "https://github.com/microsoft/vswhere/releases/download/3.1.7/vswhere.exe",
        output = "vswhere.exe",
        sha256 = "c54f3b7c9164ea9a0db8641e81ecdda80c2664ef5a47c4191406f848cc07c662",
        executable = True,
    )

    # Get identifying properties to use for reproducibility metatada
    # We stick to the version for now
    vs_version = _get_vs_property(ctx, ctx.attr.path, "installationVersion")

    # Symlink to existing installation
    ctx.symlink(ctx.attr.path, "VisualStudio")

    build_file_content = """
exports_files(["VisualStudio/MSBuild/Current/Bin/msbuild.exe"])

alias(
    name="msbuild",
    actual="VisualStudio/MSBuild/Current/Bin/msbuild.exe",
    visibility = ["//visibility:public"],
)
"""
    ctx.file("BUILD.bazel", build_file_content)

    return ctx.repo_metadata(attrs_for_reproducibility = {"version": vs_version})

def _get_vs_property(ctx, install_path, property):
    """Query a property of a VS installation using vswhere"""
    result = ctx.execute([
        ctx.path("vswhere.exe"),
        "-nologo",
        "-nocolor",
        "-property",
        property,
        "-path",
        install_path,
    ])
    if result.return_code:
        fail("Failed to query property '%s' for '%s': %s" % property, install_path, result.stderr)

    return result.stdout.strip()

visual_studio = repository_rule(
    implementation = _visual_studio_impl,
    attrs = {
        "path": attr.string(
            mandatory = True,
            doc = "Path to Visual Studio's installation root",
        ),
    },
    local = True,
    configure = True,
    doc =
        """Wraps an existing Visual Studio installation.""",
)
