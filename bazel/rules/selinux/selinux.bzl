"""compile_selinux_policy - compile a SELinux .te policy source into a packaged .pp module.

Based on: `dda inv selinux.compile-system-probe-policy-file` (tasks/selinux.py).
"""

def _compile_selinux_policy_impl(ctx):
    tools = ctx.toolchains["@selinux_tools//:selinux_tools_toolchain_type"]
    checkmodule = tools.checkmodule
    semodule_package = tools.semodule_package
    if not checkmodule.valid or not semodule_package.valid:
        fail("No SELinux policy tools (checkmodule/semodule_package) available on this machine.")

    mod_file = ctx.actions.declare_file(ctx.label.name + ".mod")
    output = ctx.outputs.out

    # checkmodule.path/semodule_package.path are plain strings pointing at
    # system binaries found via `which` (see
    # bazel/toolchains/selinux_tools/configure.bzl) -- they aren't
    # Bazel-tracked Files, so they must not be added to `inputs`.
    ctx.actions.run(
        mnemonic = "SelinuxCheckModule",
        progress_message = "Compiling SELinux policy module %s" % ctx.label,
        executable = checkmodule.path,
        arguments = ["-M", "-m", "-c", str(ctx.attr.policy_version), "-o", mod_file.path, ctx.file.src.path],
        inputs = [ctx.file.src],
        outputs = [mod_file],
    )
    ctx.actions.run(
        mnemonic = "SelinuxPackageModule",
        progress_message = "Packaging SELinux policy module %s" % ctx.label,
        executable = semodule_package.path,
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
    toolchains = ["@selinux_tools//:selinux_tools_toolchain_type"],
)
