"""A module defining a common framework for "built_tools" rules"""

load("@bazel_tools//tools/cpp:toolchain_utils.bzl", "find_cpp_toolchain")
load("//foreign_cc/private:cc_toolchain_util.bzl", "absolutize_path_in_str")
load("//foreign_cc/private:detect_root.bzl", "detect_root")
load("//foreign_cc/private:framework.bzl", "get_env_prelude", "wrap_outputs")
load("//foreign_cc/private/framework:helpers.bzl", "convert_shell_script", "shebang")
load("//foreign_cc/private/framework:platform.bzl", "PLATFORM_CONSTRAINTS_RULE_ATTRIBUTES")

# Common attributes for all built_tool rules
FOREIGN_CC_BUILT_TOOLS_ATTRS = {
    "configure_xcompile": attr.bool(
        doc = (
            "If this is set and an xcompile scenario is detected, pass the necessary autotools flags. (Only applies if autotools is used)"
        ),
        default = False,
    ),
    "env": attr.string_dict(
        doc = "Environment variables to set during the build. This attribute is subject to make variable substitution.",
        default = {},
    ),
    "srcs": attr.label(
        doc = "The target containing the build tool's sources",
        mandatory = True,
    ),
    "_cc_toolchain": attr.label(
        default = Label("@bazel_tools//tools/cpp:current_cc_toolchain"),
    ),
    "_foreign_cc_framework_platform": attr.label(
        doc = "Information about the execution platform",
        cfg = "exec",
        default = Label("@rules_foreign_cc//foreign_cc/private/framework:platform_info"),
    ),
}

# this would be cleaner as x | y, but that's not supported in bazel 5.4.0
FOREIGN_CC_BUILT_TOOLS_ATTRS.update(PLATFORM_CONSTRAINTS_RULE_ATTRIBUTES)

# Common fragments for all built_tool rules
FOREIGN_CC_BUILT_TOOLS_FRAGMENTS = [
    "apple",
    "cpp",
]

# Common host fragments for all built_tool rules
FOREIGN_CC_BUILT_TOOLS_HOST_FRAGMENTS = [
    "cpp",
]

def absolutize(workspace_name, text, force = False):
    return absolutize_path_in_str(workspace_name, "$$EXT_BUILD_ROOT$$/", text, force)

def extract_sysroot_flags(flags):
    """Function to return sysroot args from list of flags like cflags or ldflags

    sysroot args are either '--sysroot=</path/to/sysroot>' or '--sysroot </path/to/sysroot>'

    Args:
        flags (list): list of flags

    Returns:
        List of sysroot flags
    """
    ret_flags = []
    for i in range(len(flags)):
        if flags[i] == "--sysroot":
            if i + 1 < len(flags):
                ret_flags.append(flags[i])
                ret_flags.append(flags[i + 1])
        elif flags[i].startswith("--sysroot="):
            ret_flags.append(flags[i])
    return ret_flags

def extract_non_sysroot_flags(flags):
    """Function to return non sysroot args from list of flags like cflags or ldflags

    sysroot args are either '--sysroot=</path/to/sysroot>' or '--sysroot </path/to/sysroot>'

    Args:
        flags (list): list of flags

    Returns:
        List of non sysroot flags
    """
    ret_flags = []
    for i in range(len(flags)):
        if flags[i] == "--sysroot" or \
           flags[i].startswith("--sysroot=") or \
           (i != 0 and flags[i - 1] == "--sysroot"):
            continue
        else:
            ret_flags.append(flags[i])
    return ret_flags

def built_tool_rule_impl(ctx, script_lines, out_dir, mnemonic, additional_tools = None):
    """Framework function for bootstrapping C/C++ build tools.

    This macro should be shared by all "built-tool" rules defined in rules_foreign_cc.
    Any rule implementing this function should ensure that the appropriate artifacts
    are placed in a directory represented by the `INSTALLDIR` environment variable.

    Args:
        ctx (ctx): The current rule's context object
        script_lines (list): A list of lines of a bash script for building the build tool
        out_dir (File): The output directory of the build tool
        mnemonic (str): The mnemonic of the build action
        additional_tools (depset): A list of additional tools to include in the build action

    Returns:
        list: A list of providers
    """

    root = detect_root(ctx.attr.srcs)
    lib_name = ctx.attr.name
    env_prelude = get_env_prelude(ctx, out_dir.path, [], {})

    cc_toolchain = find_cpp_toolchain(ctx)

    script = [
        "##script_prelude##",
    ] + env_prelude + [
        "##rm_rf## $$INSTALLDIR$$",
        "##rm_rf## $$BUILD_TMPDIR$$",
        "##mkdirs## $$INSTALLDIR$$",
        "##mkdirs## $$BUILD_TMPDIR$$",
        "##copy_dir_contents_to_dir## ./{} $$BUILD_TMPDIR$$".format(root),
        "cd \"$$BUILD_TMPDIR$$\"",
    ]

    script.append("##enable_tracing##")
    script.extend(script_lines)
    script.append("##disable_tracing##")

    script_text = "\n".join([
        shebang(ctx),
        convert_shell_script(ctx, script),
        "",
    ])

    wrapped_outputs = wrap_outputs(
        ctx,
        lib_name = lib_name,
        configure_name = mnemonic,
        env_prelude = env_prelude,
        script_text = script_text,
    )

    tools = depset(
        [wrapped_outputs.wrapper_script_file, wrapped_outputs.script_file],
        transitive = [cc_toolchain.all_files],
    )

    if additional_tools:
        tools = depset(transitive = [tools, additional_tools])

    # The use of `run_shell` here is intended to ensure bash is correctly setup on windows
    # environments. This should not be replaced with `run` until a cross platform implementation
    # is found that guarantees bash exists or appropriately errors out.
    ctx.actions.run_shell(
        mnemonic = mnemonic,
        inputs = ctx.attr.srcs.files,
        outputs = [out_dir, wrapped_outputs.log_file],
        tools = tools,
        use_default_shell_env = True,
        command = wrapped_outputs.wrapper_script_file.path,
        execution_requirements = {"block-network": ""},
    )

    return [
        DefaultInfo(files = depset([out_dir]), runfiles = ctx.runfiles(files = [out_dir])),
        OutputGroupInfo(
            log_file = depset([wrapped_outputs.log_file]),
            script_file = depset([wrapped_outputs.script_file]),
            wrapper_script_file = depset([wrapped_outputs.wrapper_script_file]),
        ),
    ]
