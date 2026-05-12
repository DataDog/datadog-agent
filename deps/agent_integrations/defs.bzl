"""Rules for installing integrations and their dependencies."""

def _install_wheels_impl(ctx):
    requirements_file = ctx.actions.declare_file(ctx.attr.name + "_requirements.txt")

    # Create an ad-hoc requirements file (a wheel per line)
    ctx.actions.write(requirements_file, "\n".join([src.path for src in ctx.files.srcs]))

    # TODO(agent-build): consider
    # - Installing the individual wheels in separate actions for per-wheel caching.
    # - Simply unzipping the wheel instead of resorting to pip

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
            "-r",
            requirements_file.path,
            "--target",
            installation_dir.path,
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
            cfg = "exec",
            doc = """Executable target providing the python interpreter. Its
            DefaultInfo.files must contain everything needed at runtime
            (libpython, lib-dynload, dynamic deps).""",
        ),
    },
    doc = "Installs the wheels given in `srcs` into a directory using pip.",
)
