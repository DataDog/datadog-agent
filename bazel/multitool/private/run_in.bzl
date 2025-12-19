"run_in provides a shared implementation for cwd and workspace_root execution"

run_in_attrs = {
    "tool": attr.label(mandatory = True, executable = True, cfg = "exec"),
    "_template_sh": attr.label(default = "//bazel/multitool/private:run_in.template.sh", allow_single_file = True),
    "_template_bat": attr.label(default = "//bazel/multitool/private:run_in.template.bat", allow_single_file = True),
}

def run_in(ctx, env_var):
    """
    run_in implements a rule that runs a tool in a directory provided by env_var

    Args:
        ctx: rule ctx argument
        env_var: env var string that contains the desired execution directory

    Returns:
        A list including the DefaultInfo provider for the created wrapper
        executable
    """

    # This algorithm requires --enable_runfiles (enabled by default on non-windows)
    template = ctx.file._template_sh
    wrapper_name = ctx.label.name
    tool_short_path = ctx.executable.tool.short_path
    if ctx.executable.tool.extension == "exe":
        template = ctx.file._template_bat
        wrapper_name = wrapper_name + ".bat"
        tool_short_path = tool_short_path.replace("/", "\\")
    output = ctx.actions.declare_file(wrapper_name)
    ctx.actions.expand_template(
        template = template,
        output = output,
        substitutions = {
            "{{tool}}": tool_short_path,
            "{{env_var}}": env_var,
        },
    )
    return [DefaultInfo(executable = output, runfiles = ctx.attr.tool[DefaultInfo].default_runfiles)]
