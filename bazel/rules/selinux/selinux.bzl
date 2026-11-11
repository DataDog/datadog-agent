"""compile_selinux_policy - compile a SELinux .te policy source into a packaged .pp module.

`dda inv selinux.compile-system-probe-policy-file` writes this rule's output to the source tree.
"""

_SELINUX_TOOLS = "@@//bazel/toolchains/selinux_tools:selinux_tools_toolchain_type"

def _compile_selinux_policy_impl(ctx):
    tools = ctx.toolchains[_SELINUX_TOOLS]
    checkmodule = tools.checkmodule
    semodule_package = tools.semodule_package

    mod_file = ctx.actions.declare_file(ctx.label.name + ".mod")
    output = ctx.outputs.out

    ctx.actions.run(
        mnemonic = "SelinuxCheckModule",
        progress_message = "Compiling SELinux policy module %s" % ctx.label,
        executable = checkmodule,
        arguments = ["-M", "-m", "-c", str(ctx.attr.policy_version), "-o", mod_file.path, ctx.file.src.path],
        inputs = [ctx.file.src],
        outputs = [mod_file],
    )
    ctx.actions.run(
        mnemonic = "SelinuxPackageModule",
        progress_message = "Packaging SELinux policy module %s" % ctx.label,
        executable = semodule_package,
        arguments = ["-o", output.path, "-m", mod_file.path],
        inputs = [mod_file],
        outputs = [output],
    )

    return [DefaultInfo(files = depset([output]))]

compile_selinux_policy = rule(
    implementation = _compile_selinux_policy_impl,
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".te"], doc = "SELinux .te policy source."),
        "policy_version": attr.int(
            default = 19,
            doc = "checkmodule -c modular policy version to target.",
        ),
        "out": attr.output(mandatory = True, doc = "Output packaged .pp module filename."),
    },
    toolchains = [_SELINUX_TOOLS],
)
