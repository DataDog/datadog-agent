"""Compile a static i686 musl binary with the Bootlin cross gcc."""

def _musl_i686_cc_binary_impl(ctx):
    out = ctx.actions.declare_file(ctx.attr.name)
    args = ctx.actions.args()
    args.add("-static")
    args.add("-o", out)
    args.add_all(ctx.files.srcs)
    ctx.actions.run(
        executable = ctx.executable._gcc,
        arguments = [args],
        inputs = depset(ctx.files.srcs + ctx.files._prefix),
        outputs = [out],
        mnemonic = "MuslI686Compile",
        progress_message = "Compiling %{label} for i686-linux-musl",
    )
    return DefaultInfo(
        files = depset([out]),
        executable = out,
    )

musl_i686_cc_binary = rule(
    implementation = _musl_i686_cc_binary_impl,
    attrs = {
        "srcs": attr.label_list(
            allow_files = [".c"],
            mandatory = True,
        ),
        "_gcc": attr.label(
            default = "@bootlin_i686_musl//:bin/i686-linux-gcc",
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
        "_prefix": attr.label(
            default = "@bootlin_i686_musl//:prefix",
            cfg = "exec",
        ),
    },
    executable = True,
    exec_compatible_with = [
        "@platforms//cpu:x86_64",
        "@platforms//os:linux",
    ],
)
