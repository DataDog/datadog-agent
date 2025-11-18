"""A module defining convienence methoods for accessing build tools from
rules_foreign_cc toolchains
"""

def access_tool(toolchain_type_, ctx):
    """A helper macro for getting the path to a build tool's executable

    Args:
        toolchain_type_ (Label): The name of the toolchain type
        ctx (ctx): The rule's context object

    Returns:
        ToolInfo: A provider containing information about the toolchain's executable
    """
    tool_toolchain = ctx.toolchains[toolchain_type_]
    if tool_toolchain:
        return tool_toolchain.data
    fail("No toolchain found for " + toolchain_type_)

def get_autoconf_data(ctx):
    return _access_and_expect_label_copied(Label("//toolchains:autoconf_toolchain"), ctx)

def get_automake_data(ctx):
    return _access_and_expect_label_copied(Label("//toolchains:automake_toolchain"), ctx)

def get_cmake_data(ctx):
    return _access_and_expect_label_copied(Label("//toolchains:cmake_toolchain"), ctx)

def get_m4_data(ctx):
    return _access_and_expect_label_copied(Label("//toolchains:m4_toolchain"), ctx)

def get_make_data(ctx):
    return _access_and_expect_label_copied(Label("//toolchains:make_toolchain"), ctx)

def get_ninja_data(ctx):
    return _access_and_expect_label_copied(Label("//toolchains:ninja_toolchain"), ctx)

def get_meson_data(ctx):
    return _access_and_expect_label_copied(Label("//toolchains:meson_toolchain"), ctx)

def get_pkgconfig_data(ctx):
    return _access_and_expect_label_copied(Label("//toolchains:pkgconfig_toolchain"), ctx)

def _access_and_expect_label_copied(toolchain_type_, ctx):
    tool_data = access_tool(toolchain_type_, ctx)
    if tool_data.target:
        # This could be made more efficient by changing the
        # toolchain to provide the executable as a target
        cmd_file = tool_data
        tool_env = dict(tool_data.env)

        for f in tool_data.target.files.to_list():
            if f.path.endswith("/" + tool_data.path):
                cmd_file = f
                break

        # Environment vars for tools such as MAKE and CMAKE needs to be absolute
        # as they are used from build_tmpdir and not bazel's exec/sandbox root
        for k, v in tool_env.items():
            if v.endswith(tool_data.path):
                tool_env[k] = "$EXT_BUILD_ROOT/{}".format(cmd_file.path)

        return struct(
            target = tool_data.target,
            env = tool_env,
            # as the tool will be copied into tools directory
            path = "$EXT_BUILD_ROOT/{}".format(cmd_file.path),
        )
    else:
        return struct(
            target = None,
            env = tool_data.env,
            path = tool_data.path,
        )
