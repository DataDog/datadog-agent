"""Convert TestRunner test.log outputs to test2json fragments via aspect actions."""

def _fragment_basename(target_name, test_log_path):
    """Basename of the fragment for one TestRunner test.log path.

    A target can own several TestRunner actions (test sharding, --runs_per_test),
    so the tail of the test.log path is folded into the name to keep the declared
    files unique.
    """
    marker = target_name + "/"
    idx = test_log_path.find(marker)
    if idx >= 0:
        suffix = test_log_path[idx + len(marker):]
    else:
        suffix = test_log_path.split("/")[-1]
    safe = suffix.replace("/", "_").replace(":", "_")
    return target_name + "_" + safe + ".test2json.jsonl"

def _label_string(label):
    # Deliberately not str(label): in the main repo that renders as
    # "@@//pkg/foo:foo_test", and the "@@" would leak into the test2json Package
    # field that downstream tooling keys on.
    return "//%s:%s" % (label.package, label.name)

def _test_log_to_json_impl(target, ctx):
    label_str = _label_string(target.label)
    fragments = []
    for action in target.actions:
        if action.mnemonic != "TestRunner":
            continue
        for test_log in [f for f in action.outputs.to_list() if f.basename == "test.log"]:
            out = ctx.actions.declare_file(_fragment_basename(target.label.name, test_log.path))
            args = ctx.actions.args()
            args.add("-label", label_str)
            args.add("-log", test_log)
            args.add("-output", out)
            ctx.actions.run(
                executable = ctx.executable._converter,
                inputs = [test_log],
                outputs = [out],
                arguments = [args],
                mnemonic = "TestLogToJson",
            )
            fragments.append(out)
    if not fragments:
        return []

    # The group name is also spelled in bazel/configs/go_tests.bazelrc
    # (--output_groups=+test2json) and in tasks/bazel.py, which reads it back
    # out of the BEP.
    return [OutputGroupInfo(test2json = depset(fragments))]

test_log_to_json = aspect(
    doc = "Runs testlogs_to_json on each TestRunner test.log for this target.",
    implementation = _test_log_to_json_impl,
    attrs = {
        "_converter": attr.label(
            default = "//bazel/tools/testlogs_to_json",
            executable = True,
            cfg = "exec",
        ),
    },
)
