"""Rules for installing integrations and their dependencies."""

def _install_wheels_impl(ctx):
    requirements_file = ctx.actions.declare_file(ctx.attr.name + "_requirements.txt")

    # Create an ad-hoc requirements file (a wheel per line)
    ctx.actions.write(requirements_file, "\n".join([src.path for src in ctx.files.srcs]))

    # TODO(agent-build): consider
    # - Installing the individual wheels in separate actions for per-wheel caching.
    # - Simply unzipping the wheels instead of resorting to pip

    installation_dir = ctx.actions.declare_directory(ctx.attr.output or ctx.attr.name)

    ctx.actions.run(
        mnemonic = "InstallPythonWheels",
        inputs = ctx.files.srcs + [requirements_file],
        outputs = [installation_dir],
        executable = ctx.attr.python[DefaultInfo].files_to_run,
        arguments = [
            "-m",
            "pip",
            "install",
            "--only-binary=:all:",
            "--no-deps",
            "--no-index",
            "--prefix",
            installation_dir.path,
            "-r",
            requirements_file.path,
        ],
        progress_message = "Installing Python wheels for %{label}",
    )

    return DefaultInfo(
        files = depset([installation_dir]),
    )

install_wheels = rule(
    implementation = _install_wheels_impl,
    attrs = {
        "srcs": attr.label_list(allow_files = [".whl"]),
        "output": attr.string(
            doc = "Name of the output directory. Defaults to the rule name.",
        ),
        "python": attr.label(
            mandatory = True,
            executable = True,
            # Even though it's a tool, which should usually require cfg "exec" (to enable
            # cross-build), in practice the whole point of using our own-built python here
            # is that it needs to match our target, and using cfg "exec" would require a separate
            # rebuild of Python.
            # We can revisit this once all of the pieces related to integrations-core are migrated
            # to dig into whether we do actually need our embedded Python or we can do without.
            cfg = "target",
            doc = """Executable target providing the python interpreter, ideally matching the target platform.""",
        ),
    },
    doc = "Installs the wheels given in `srcs` into a directory using pip.",
)
