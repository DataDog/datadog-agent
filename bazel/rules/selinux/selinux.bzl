"""compile_selinux_policy - compile a SELinux .te policy source into a packaged .pp module.

Wraps `checkmodule`/`semodule_package` via the @selinux_tools toolchain (see
bazel/toolchains/selinux_tools/configure.bzl), mirroring
`dda inv selinux.compile-system-probe-policy-file` (tasks/selinux.py).
"""

def _compile_selinux_policy_impl(ctx):
    tools = ctx.toolchains["@selinux_tools//:selinux_tools_toolchain_type"]
    checkmodule = tools.checkmodule
    semodule_package = tools.semodule_package
    if not checkmodule.valid or not semodule_package.valid:
        fail("No SELinux policy tools (checkmodule/semodule_package) available on this machine.")

    output = ctx.outputs.out

    args = ctx.actions.args()
    args.add("--checkmodule", checkmodule.path)
    args.add("--semodule-package", semodule_package.path)
    args.add("--policy-version", ctx.attr.policy_version)
    args.add("--te-file", ctx.file.src)
    args.add("--output", output)

    # checkmodule.path/semodule_package.path are plain strings pointing at
    # system binaries found via `which` (see
    # bazel/toolchains/selinux_tools/configure.bzl) -- they aren't
    # Bazel-tracked Files, so they must not be added to `inputs`.
    ctx.actions.run(
        mnemonic = "SelinuxCompilePolicy",
        progress_message = "Compiling SELinux policy %s" % ctx.label,
        executable = ctx.executable._wrapper,
        arguments = [args],
        inputs = [ctx.file.src],
        outputs = [output],
    )

    return [DefaultInfo(files = depset([output]))]

compile_selinux_policy = rule(
    implementation = _compile_selinux_policy_impl,
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".te"], doc = "SELinux .te policy source."),
        "policy_version": attr.string(
            default = "19",
            doc = "checkmodule -c modular policy version to target.",
        ),
        "out": attr.output(mandatory = True, doc = "Output packaged .pp module filename."),
        "_wrapper": attr.label(
            default = Label("//bazel/rules/selinux:compile_selinux_policy"),
            executable = True,
            cfg = "exec",
        ),
    },
    toolchains = ["@selinux_tools//:selinux_tools_toolchain_type"],
)
