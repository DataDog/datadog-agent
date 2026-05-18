"""Rule to compute MD5 checksums of files within a tar archive."""

def _tar_md5sums_impl(ctx):
    output = ctx.outputs.md5sums
    ctx.actions.run(
        inputs = [ctx.file.src],
        outputs = [output],
        executable = ctx.executable._tool,
        arguments = [ctx.file.src.path, output.path],
    )
    return [DefaultInfo(files = depset([output]))]

tar_md5sums = rule(
    implementation = _tar_md5sums_impl,
    attrs = {
        "src": attr.label(
            mandatory = True,
            allow_single_file = True,
        ),
        "md5sums": attr.output(
            mandatory = True,
        ),
        "_tool": attr.label(
            default = "//tools/tar_checksums:tar_checksums",
            executable = True,
            cfg = "exec",
        ),
    },
)
