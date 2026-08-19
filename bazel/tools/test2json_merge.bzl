"""Rules that merge aspect test2json fragments into one JSONL file."""

load(":test2json.bzl", "Test2JsonInfo")
load(":test_log_aspect.bzl", "test_log_to_json")

def _concat_fragments(ctx, fragments, output):
    args = ctx.actions.args()
    args.add("-concat")
    args.add("-output", output)
    args.add_all(fragments, format_each = "-fragment=%s")
    ctx.actions.run(
        executable = ctx.executable._merger,
        inputs = fragments,
        outputs = [output],
        arguments = [args],
        mnemonic = "Test2JsonMerge",
    )
    return [DefaultInfo(files = depset([output]))]

def _test2json_merge_impl(ctx):
    fragments = []
    for target in ctx.attr.tests:
        info = target[Test2JsonInfo]
        fragments.extend(info.fragments.to_list())
    if not fragments:
        fail("test2json_merge: no Test2JsonInfo fragments from tests attribute")
    out = ctx.actions.declare_file("test_output.jsonl")
    return _concat_fragments(ctx, fragments, out)

test2json_merge = rule(
    doc = "Concatenate test2json aspect fragments from explicit test targets.",
    implementation = _test2json_merge_impl,
    attrs = {
        "tests": attr.label_list(
            doc = "Test targets that were built with the test_log_to_json aspect.",
            aspects = [test_log_to_json],
            providers = [Test2JsonInfo],
        ),
        "_merger": attr.label(
            default = "//bazel/tools/testlogs_to_json",
            executable = True,
            cfg = "exec",
        ),
    },
)

def _test2json_merge_from_bep_impl(ctx):
    out = ctx.actions.declare_file("test_output.jsonl")
    args = ctx.actions.args()
    args.add("-merge-bep", ctx.file.bep)
    args.add("-output", out)
    ctx.actions.run(
        executable = ctx.executable._merger,
        inputs = [ctx.file.bep],
        outputs = [out],
        arguments = [args],
        mnemonic = "Test2JsonMerge",
        # Fragment paths are resolved from BEP + bazel-out at execution time.
        execution_requirements = {"no-sandbox": "1"},
    )
    return [DefaultInfo(files = depset([out]))]

test2json_merge_from_bep = rule(
    doc = "Merge test2json aspect fragments listed in a Bazel BEP JSONL file.",
    implementation = _test2json_merge_from_bep_impl,
    attrs = {
        "bep": attr.label(
            mandatory = True,
            allow_single_file = [".json"],
        ),
        "_merger": attr.label(
            default = "//bazel/tools/testlogs_to_json",
            executable = True,
            cfg = "exec",
        ),
    },
)
