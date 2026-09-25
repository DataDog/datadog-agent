"""Exercises a real formatting action without modifying source files."""

load("//bazel/dotslash:defs.bzl", "DOTSLASH_EXEC_GROUP", "DotSlashToolInfo", "dotslash_run")

def _format_fixture_impl(ctx):
    output = ctx.actions.declare_file(ctx.label.name + ".txt")
    dotslash_run(
        ctx,
        tool = ctx.attr.tool,
        outputs = [output],
        inputs = depset([ctx.file.src]),
        # Give stdin a diagnostic path that exposes accidental shell expansion.
        arguments = ["-config=off", "-lint=off", "-type=bzl", "-mode=fix", "-path=file %DOTSLASH_LITERAL% & space.bzl", "-"],
        env = {"FIXTURE_INPUT": ctx.file.src.path, "FIXTURE_OUTPUT": output.path},
        wrapper = ctx.attr._capture[DefaultInfo].files_to_run,
        mnemonic = "DotSlashFixture",
    )
    return [DefaultInfo(files = depset([output]))]

format_fixture = rule(
    implementation = _format_fixture_impl,
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = True),
        "tool": attr.label(mandatory = True, providers = [DotSlashToolInfo]),
        "_capture": attr.label(default = ":capture", executable = True, cfg = config.exec("dotslash")),
    },
    exec_groups = {"dotslash": DOTSLASH_EXEC_GROUP},
)
