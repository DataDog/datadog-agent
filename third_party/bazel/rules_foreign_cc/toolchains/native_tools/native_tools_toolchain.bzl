"""Rules for building native build tools such as ninja, make or cmake"""

# buildifier: disable=module-docstring
ToolInfo = provider(
    doc = "Information about the native tool",
    fields = {
        "env": "Environment variables to set when using this tool e.g. M4",
        "path": (
            "Absolute path to the tool in case the tool is preinstalled on the machine. " +
            "Relative path to the tool in case the tool is built as part of a build; the path should be relative " +
            "to the bazel-genfiles, i.e. it should start with the name of the top directory of the built tree " +
            "artifact. (Please see the example `//examples:built_cmake_toolchain`)"
        ),
        "target": (
            "If the tool is preinstalled, must be None. " +
            "If the tool is built as part of the build, the corresponding build target, which should produce " +
            "the tree artifact with the binary to call."
        ),
    },
)

def _resolve_tool_path(ctx, path, target, tools):
    """
        Resolve the path to a tool.

        Note that ctx.resolve_command is used instead of ctx.expand_location as the
        latter cannot be used with py_binary and sh_binary targets as they both produce multiple files in some contexts, meaning
        that the plural make variables must be used, e.g.  $(execpaths) must be used. See https://github.com/bazelbuild/bazel/issues/11820.

        The usage of ctx.resolve_command facilitates the usage of the singular make variables, e.g $(execpath), with py_binary and sh_binary targets
    """
    _, resolved_bash_command, _ = ctx.resolve_command(
        command = path,
        expand_locations = True,
        tools = tools + [target],
    )

    return resolved_bash_command[-1]

def _native_tool_toolchain_impl(ctx):
    if not ctx.attr.path and not ctx.attr.target:
        fail("Either path or target (and path) should be defined for the tool.")
    path = None
    env = {}
    if ctx.attr.target:
        path = _resolve_tool_path(ctx, ctx.attr.path, ctx.attr.target, ctx.attr.tools)

        for k, v in ctx.attr.env.items():
            env[k] = _resolve_tool_path(ctx, v, ctx.attr.target, ctx.attr.tools)

    else:
        path = ctx.expand_location(ctx.attr.path)
        env = {k: ctx.expand_location(v) for (k, v) in ctx.attr.env.items()}
    return platform_common.ToolchainInfo(data = ToolInfo(
        env = env,
        path = path,
        target = ctx.attr.target,
    ))

native_tool_toolchain = rule(
    doc = (
        "Rule for defining the toolchain data of the native tools (cmake, ninja), " +
        "to be used by rules_foreign_cc with toolchain types " +
        "`@rules_foreign_cc//toolchains:cmake_toolchain` and " +
        "`@rules_foreign_cc//toolchains:ninja_toolchain`."
    ),
    implementation = _native_tool_toolchain_impl,
    attrs = {
        "env": attr.string_dict(
            doc = "Environment variables to be set when using this tool e.g. M4",
        ),
        "path": attr.string(
            mandatory = False,
            doc = (
                "Absolute path to the tool in case the tool is preinstalled on the machine. " +
                "Relative path to the tool in case the tool is built as part of a build; the path should be " +
                "relative to the bazel-genfiles, i.e. it should start with the name of the top directory " +
                "of the built tree artifact. (Please see the example `//examples:built_cmake_toolchain`)"
            ),
        ),
        "target": attr.label(
            mandatory = False,
            cfg = "exec",
            doc = (
                "If the tool is preinstalled, must be None. " +
                "If the tool is built as part of the build, the corresponding build target, " +
                "which should produce the tree artifact with the binary to call."
            ),
            allow_files = True,
        ),
        "tools": attr.label_list(
            mandatory = False,
            cfg = "exec",
            doc = (
                "Additional tools." +
                "If `target` expands to several files, `tools` can be used to " +
                "isolate a specific file that can be used in `env`."
            ),
            allow_files = True,
        ),
    },
)
