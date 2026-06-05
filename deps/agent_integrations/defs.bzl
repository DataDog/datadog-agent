"""Rules for installing integrations and their dependencies."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@cpython_versions//:constants.bzl", "PYTHON_MAJOR_MINOR")

def _install_wheels_impl(ctx):
    output = ctx.attr.output or ctx.attr.name
    runtime_dir = ctx.actions.declare_directory(output + "_runtime")
    bin_dir = ctx.actions.declare_directory(output + "_bin")

    args = ctx.actions.args()
    install_dir = ctx.attr.install_dir[BuildSettingInfo].value.rstrip("/")
    script_interpreter = install_dir + "/" + ctx.attr.script_interpreter_relative_path.lstrip("/")

    args.add("--runtime-output", runtime_dir.path)
    args.add("--bin-output", bin_dir.path)
    args.add("--python-version", ctx.attr.python_version)
    args.add("--interpreter", script_interpreter)
    args.add("--script-kind", ctx.attr.script_kind)
    args.add_all(ctx.files.srcs)

    ctx.actions.run(
        mnemonic = "InstallPythonWheels",
        inputs = ctx.files.srcs,
        outputs = [runtime_dir, bin_dir],
        executable = ctx.executable._installer,
        arguments = [args],
        progress_message = "Installing Python wheels for %{label}",
    )

    return [
        DefaultInfo(
            files = depset([runtime_dir, bin_dir]),
        ),
        OutputGroupInfo(
            runtime = depset([runtime_dir]),
            bin = depset([bin_dir]),
        ),
    ]

install_wheels = rule(
    implementation = _install_wheels_impl,
    attrs = {
        "srcs": attr.label_list(allow_files = [".whl"]),
        "output": attr.string(
            doc = "Name of the output directory. Defaults to the rule name.",
        ),
        "python_version": attr.string(
            default = PYTHON_MAJOR_MINOR,
            doc = "Target Python major.minor version, used to compute the site-packages path.",
        ),
        "install_dir": attr.label(
            default = Label("//:install_dir"),
        ),
        "script_interpreter_relative_path": attr.string(
            mandatory = True,
            doc = "Interpreter path relative to `install_dir`.",
        ),
        "script_kind": attr.string(
            default = "posix",
            doc = "Script kind passed to PyPA installer for generated entry-point scripts.",
        ),
        "_installer": attr.label(
            default = ":install_wheels_tool",
            executable = True,
            cfg = "exec",
            doc = "Executable target for the wheel installation tool.",
        ),
    },
    doc = "Installs the wheels given in `srcs` into the Agent embedded Python layout.",
)
