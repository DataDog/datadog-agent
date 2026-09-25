"""Declares manifest-defined tools and invokes the selected DotSlash runtime."""

_TOOLCHAIN_TYPE = Label("//bazel/dotslash:toolchain_type")

DotSlashToolInfo = provider(
    doc = "Exposes a checked-in DotSlash manifest as a File without selecting its runtime.",
    fields = ["manifest"],
)

DOTSLASH_EXEC_GROUP = exec_group(toolchains = [_TOOLCHAIN_TYPE])

def _dotslash_tool_impl(ctx):
    return [
        DotSlashToolInfo(manifest = ctx.file.manifest),
        DefaultInfo(files = depset([ctx.file.manifest])),
    ]

dotslash_tool = rule(
    implementation = _dotslash_tool_impl,
    attrs = {"manifest": attr.label(mandatory = True, allow_single_file = True)},
)

def dotslash_run(ctx, tool, outputs, arguments = [], inputs = depset(), env = {}, wrapper = None, **kwargs):
    """Registers an action with the selected runtime's inputs and execution policy.

    The calling rule must declare a `dotslash` exec group using DOTSLASH_EXEC_GROUP.
    Manifest targets do not require an execution transition. An optional
    wrapper receives the DotSlash executable, manifest, and tool arguments as its argv.

    Args:
        ctx: Supply the context of the rule declaring the action.
        tool: Supply a target providing DotSlashToolInfo.
        outputs: Declare the files the action produces.
        arguments: Pass strings or Args objects through to the tool.
        inputs: Include the tool's input files as a depset.
        env: Add environment variables without overriding the runtime's values.
        wrapper: Optionally supply a FilesToRunProvider to wrap the invocation.
        **kwargs: Forward additional parameters to ctx.actions.run.
    """
    runtime = ctx.exec_groups["dotslash"].toolchains[_TOOLCHAIN_TYPE]
    manifest = tool[DotSlashToolInfo].manifest
    prefix = ctx.actions.args()
    if wrapper:
        prefix.add(runtime.executable)
    prefix.add(manifest)
    ctx.actions.run(
        executable = wrapper or runtime.executable,
        arguments = [prefix] + arguments,
        inputs = depset([manifest], transitive = [inputs, runtime.inputs]),
        tools = runtime.tools,
        outputs = outputs,
        env = dict(env, **runtime.env),
        use_default_shell_env = runtime.use_default_shell_env,
        execution_requirements = runtime.execution_requirements,
        exec_group = "dotslash",
        toolchain = _TOOLCHAIN_TYPE,
        **kwargs
    )
