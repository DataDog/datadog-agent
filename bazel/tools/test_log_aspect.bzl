"""Convert TestRunner test.log outputs to test2json fragments via aspect actions."""

_TEST_LOG = "test.log"
_OUTPUT_GROUP = "test2json"

def _fragment_name(target, test_log):
    label_name = target.label.name
    log_path = test_log.path
    marker = label_name + "/"
    idx = log_path.find(marker)
    if idx >= 0:
        suffix = log_path[idx + len(marker):]
    else:
        suffix = test_log.basename
    safe = suffix.replace("/", "_").replace(":", "_")
    return label_name + "_" + safe + ".test2json.jsonl"

def _test_log_paths(test_runner_action):
    return [
        f
        for f in test_runner_action.outputs.to_list()
        if f.basename == _TEST_LOG
    ]

def _label_string(label):
    return "//%s:%s" % (label.package, label.name)

def _test_log_to_json_impl(target, ctx):
    label_str = _label_string(target.label)
    fragments = []
    for action in target.actions:
        if action.mnemonic != "TestRunner":
            continue
        for test_log in _test_log_paths(action):
            out = ctx.actions.declare_file(_fragment_name(target, test_log))
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
    return [OutputGroupInfo(**{_OUTPUT_GROUP: depset(fragments)})]

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
