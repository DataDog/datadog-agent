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

    if ctx.attr.path and ctx.attr.path_variable:
        fail("Only one of `path` or `path_variable` can be set, not both")

    if not (ctx.attr.path or ctx.attr.path_variable):
        fail("One of `path` or `path_variable` need to be set")

    if ctx.attr.path_variable:
        vs_path = ctx.getenv(ctx.attr.path_variable)
        if not vs_path:
            fail("Environment variable '%s' is not defined" % ctx.attr.path_variable)
    else:
        vs_path = ctx.attr.path

    # Get identifying properties to use for reproducibility metatada
    # We stick to just the version for now
    vs_version = _get_vs_property(ctx, vs_path, "installationVersion")

    if not vs_version:
        fail(
            "Version couldn't be detected for '%s'. This probably means there is no VS installation for the provided path." %
            vs_path,
        )

    if ctx.attr.version and ctx.attr.version != vs_version:
        fail("Version '%s' doesn't match expected version '%s'" % (vs_version, ctx.attr.version))

    # Symlink to existing installation
    ctx.symlink(vs_path, "VisualStudio")

    build_file_content = """
exports_files(["VisualStudio/MSBuild/Current/Bin/msbuild.exe"])

alias(
    name="msbuild",
    actual="VisualStudio/MSBuild/Current/Bin/msbuild.exe",
    visibility = ["//visibility:public"],
)
"""
    ctx.file("BUILD.bazel", build_file_content)

    # We make a pragmatic choice around how we report reproducibility by greatly relaxing assumptions
    if ctx.attr.version and ctx.attr.path:
        return ctx.repo_metadata(reproducible = True)

    return ctx.repo_metadata(attrs_for_reproducibility = {
        "name": ctx.attr.name,
        "version": vs_version,
        "path": vs_path,
    })

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
            doc = "Path to Visual Studio's installation root",
        ),
        "path_variable": attr.string(
            doc = "Environment variable pointing to Visual Studio's installation root path",
        ),
        "version": attr.string(
            doc = "Installation Version. If set, it must match the version for the installation pointed at by path",
        ),
    },
    local = True,
    configure = True,
    doc =
        """Wraps an existing Visual Studio installation.""",
)
