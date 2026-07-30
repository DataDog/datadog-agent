"""Rules for installing integrations and their dependencies."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@cpython_versions//:constants.bzl", "PYTHON_MAJOR_MINOR")

def _pyproject_wheel_impl(ctx):
    # The output of the rule is a directory containing the wheel.
    # This avoids having to figure out the appropriate wheel name that the build will produce.
    output = ctx.attr.output or ctx.attr.name
    wheel_dir = ctx.actions.declare_directory(output)

    inputs = depset(ctx.files.srcs + [ctx.file.pyproject])

    args = ctx.actions.args()
    args.add("--src", ctx.file.pyproject.dirname)
    args.add("--output-dir", wheel_dir.path)
    args.add_all(ctx.attr.exclude, before_each = "--exclude")

    ctx.actions.run(
        mnemonic = "BuildPythonWheel",
        inputs = inputs,
        outputs = [wheel_dir],
        executable = ctx.executable._builder,
        arguments = [args],
        progress_message = "Building Python wheel for %{label}",
    )

    return DefaultInfo(files = depset([wheel_dir]))

def _install_wheels_impl(ctx):
    output = ctx.attr.output or ctx.attr.name
    runtime_dir = ctx.actions.declare_directory(output + "_runtime")
    bin_dir = ctx.actions.declare_directory(output + "_bin")

    args = ctx.actions.args()
    install_dir = ctx.attr.install_dir[BuildSettingInfo].value.rstrip("/")
    script_interpreter = install_dir + "/" + ctx.attr.script_interpreter_relative_path.lstrip("/")
    platform = "windows" if _is_windows_target(ctx) else "posix"

    args.add("--runtime-output", runtime_dir.path)
    args.add("--bin-output", bin_dir.path)
    args.add("--entrypoints-dirname", ctx.attr.entrypoints_dirname)
    args.add("--python-version", ctx.attr.python_version)
    args.add("--interpreter", script_interpreter)
    args.add("--platform", platform)
    args.add_all(ctx.files.srcs)
    args.use_param_file("@%s", use_always = True)
    args.set_param_file_format("multiline")

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

def _is_windows_target(ctx):
    return ctx.target_platform_has_constraint(ctx.attr._windows_constraint[platform_common.ConstraintValueInfo])

pyproject_wheel = rule(
    implementation = _pyproject_wheel_impl,
    attrs = {
        "srcs": attr.label_list(allow_files = True, mandatory = True),
        "pyproject": attr.label(
            allow_single_file = True,
            mandatory = True,
            doc = "Acts as an anchor to reference the source directory",
        ),
        "output": attr.string(
            doc = "Name of the output wheel directory. Defaults to the rule name.",
        ),
        "exclude": attr.string_list(
            doc = "Additional hatchling wheel-target exclusion patterns to apply while building the wheel.",
        ),
        "_builder": attr.label(
            default = ":build_wheel_tool",
            executable = True,
            cfg = "exec",
            doc = "Executable target for the wheel build frontend.",
        ),
    },
    doc = "Builds a wheel from a pyproject.toml-based, hatchling-built Python source package.",
)

install_wheels = rule(
    implementation = _install_wheels_impl,
    attrs = {
        "srcs": attr.label_list(allow_files = [".whl"]),
        "output": attr.string(
            doc = "Name of the output directory. Defaults to the rule name.",
        ),
        "entrypoints_dirname": attr.string(
            mandatory = True,
            doc = """Folder name where entry points are to be installed.
            This is used so that relative paths to them on RECORD entries are accurate.
            """,
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
        "_installer": attr.label(
            default = ":install_wheels_tool",
            executable = True,
            cfg = "exec",
            doc = "Executable target for the wheel installation tool.",
        ),
        "_windows_constraint": attr.label(default = "@platforms//os:windows"),
    },
    doc = "Installs the wheels given in `srcs` into the Agent embedded Python layout.",
)
