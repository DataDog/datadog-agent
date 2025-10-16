"""Rule for testing generated files against golden files."""

def _golden_file_test_impl(ctx):
    """Implementation of golden_file_test rule."""

    # Create a test script that diffs the files
    test_script = ctx.actions.declare_file(ctx.label.name + "_test.sh")
    ctx.actions.write(
        output = test_script,
        content = """#!/bin/bash -e
if diff -u "{target}" "{golden}"; then
    echo 'PASS: {name}'
    exit 0
else
    echo 'FAIL: {name}'
    echo '  To update the source do:'
    echo '  cp "bazel-bin/{target}" "{golden}"'
    exit 1
fi
""".format(
            name = ctx.label.name,
            target = ctx.file.target.short_path,
            golden = ctx.file.golden.short_path,
        ),
        is_executable = True,
    )
    return [
        DefaultInfo(
            executable = test_script,
            files = depset([ctx.file.target, ctx.file.golden]),
            runfiles = ctx.runfiles(files = [ctx.file.target, ctx.file.golden]),
        ),
    ]

golden_file_test = rule(
    implementation = _golden_file_test_impl,
    test = True,
    attrs = {
        "golden": attr.label(
            allow_single_file = True,
            mandatory = True,
            doc = "Golden file to compare against",
        ),
        "target": attr.label(
            allow_single_file = True,
            mandatory = True,
            doc = "Generated file to test",
        ),
    },
    doc = "Test that a generated file matches a golden file",
)
