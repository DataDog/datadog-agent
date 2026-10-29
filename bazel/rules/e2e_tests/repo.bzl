"""Generates `bazel run @e2e_tests//tests[/<directory>]` targets, running the E2E tests beneath in parallel.

Each package of the generated repo mirrors a `test/new-e2e/tests` directory, declaring:
- a `command` per E2E test it holds, running it through `test2json`,
- a `<directory>_tests` `multirun` of the commands beneath it,
- an eponymous `command` running that `multirun` through `scrub`, then `gotestsum`, configured by `GOTESTSUM_*` env
  vars.

An E2E test is a `go_test` of `test/new-e2e/tests` depending on `e2e`, where `e2e.Run` lives, through `test/new-e2e`
targets, as `kind(go_test, rdeps(//test/new-e2e/tests/..., //test/e2e-framework/testing/e2e))` would find.
"""

load("@rules_python//python/private:repo_utils.bzl", "repo_utils")  # buildifier: disable=bzl-visibility

_E2E = "test/e2e-framework/testing/e2e:e2e"
_PARENT = "test/new-e2e"

def _rules(package, build):
    """Maps the targets of a buildifier-formatted `BUILD.bazel` file to their rule kind and the labels they mention."""
    rules = {}
    kind, name, labels = None, None, []
    for line in build.splitlines():
        if kind == None:
            if line.endswith("(") and not line.startswith(" "):
                kind, name, labels = line.removesuffix("("), None, []
        elif line == ")":
            if name:
                rules["{}:{}".format(package, name)] = struct(kind = kind, labels = labels)
            kind = None
        else:
            strings = line.split('"')[1::2]
            if line.lstrip().startswith("name = "):
                name = strings[0]
            for string in strings:
                if string.startswith(":"):
                    labels.append(package + string)
                elif string.startswith("//"):
                    target, _, target_name = string.removeprefix("//").partition(":")
                    labels.append("{}:{}".format(target, target_name or target.rpartition("/")[2]))
    return rules

def _e2e_tests(rctx):
    """Lists the labels, without `//`, of the E2E tests."""
    rules = {}
    pending = [_PARENT]
    for _ in range(1 << 20):  # Starlark has no while loop
        if not pending:
            break
        package = pending.pop()
        for child in rctx.workspace_root.get_child(package).readdir():
            if child.is_dir and not child.basename.startswith("."):
                pending.append("{}/{}".format(package, child.basename))
            elif child.basename == "BUILD.bazel":
                rules |= _rules(package, rctx.read(child))
    reaching = {_E2E: True}
    for _ in range(len(rules)):  # fixpoint, Starlark having no recursion
        found = [
            target
            for target, rule in rules.items()
            if target not in reaching and any([label in reaching for label in rule.labels])
        ]
        if not found:
            break
        reaching |= {target: True for target in found}
    return sorted([
        target
        for target in reaching
        if target.startswith(_PARENT + "/tests/") and rules[target].kind in ("dd_agent_go_test", "go_test")
    ])

def _e2e_tests_impl(rctx):
    rctx.watch_tree(rctx.workspace_root.get_child(_PARENT))
    runs = {}
    multiruns = {}
    for test in _e2e_tests(rctx):
        package, _, name = test.partition(":")
        directory = package.removeprefix(_PARENT + "/")
        runs.setdefault(directory, []).append((name, test))
        parts = directory.split("/")
        for i in range(1, len(parts) + 1):
            multiruns.setdefault("/".join(parts[:i]), []).append("//{}:{}".format(directory, name))

    rctx.file("BUILD.bazel", "")
    for directory, commands in multiruns.items():
        content = [
            'load("@rules_multirun//:defs.bzl", "command", "multirun")',
            "",
            'package(default_visibility = ["//:__subpackages__"])',
            "",
        ]
        for name, test in runs.get(directory, []):
            content += [
                "command(",
                '    name = "{}",'.format(name),
                "    testonly = True,",
                "    arguments = {},".format(json.encode([
                    "-p",
                    "github.com/DataDog/datadog-agent/" + test.partition(":")[0],
                    "-t",
                    "$$(rlocation $(rlocationpath @@//{}))".format(test),  # resolved at run time
                    "-test.v=test2json",
                ])),
                '    command = "@test2json",',
                '    data = ["@@//{}"],'.format(test),
                "    environment = {},".format(json.encode({
                    "GO_TEST_RUN_FROM_BAZEL": "1",  # like `go test`, rules_go's test main then chdirs to the package
                    "TEST_SRCDIR": "$$(rlocation _main)/..",
                    "TEST_WORKSPACE": "_main",
                })),
                ")",
                "",
            ]
        tests = "{}_tests".format(directory.rpartition("/")[2])
        content += [
            "multirun(",
            '    name = "{}",'.format(tests),
            "    testonly = True,",
            "    commands = {},".format(json.encode(sorted(commands))),
            "    jobs = 0,",
            "    print_command = False,",
            ")",
            "",
            "command(",
            '    name = "{}",'.format(directory.rpartition("/")[2]),
            "    testonly = True,",
            "    arguments = {},".format(json.encode([
                "--raw-command",
                "--",
                "$$(rlocation $(rlocationpath @@//test/e2e-framework/cmd/scrub))",
                "$$(rlocation $(rlocationpath :{}))".format(tests),
            ])),
            '    command = "@tools_gotest_gotestsum//:gotestsum",',
            '    data = ["@@//test/e2e-framework/cmd/scrub", ":{}"],'.format(tests),
            ")",
        ]
        rctx.file("{}/BUILD.bazel".format(directory), "\n".join(content) + "\n")

e2e_tests = repository_rule(implementation = _e2e_tests_impl)

def _test2json_impl(rctx):
    sdk = rctx.path(rctx.attr._go_sdk).dirname
    rctx.watch(sdk.get_child("VERSION"))  # rebuild on SDK bump
    exe = ".exe" if repo_utils.get_platforms_os_name(rctx) == "windows" else ""
    out = "{}.exe".format(rctx.original_name)  # like native_binary, since .exe is needed on Windows, otherwise harmless
    repo_utils.execute_checked(
        rctx,
        arguments = [sdk.get_child("bin/go{}".format(exe)), "build", "-o", out, "cmd/test2json"],
        environment = {"GOROOT": str(sdk), "GOWORK": "off"},
        op = "build {}".format(out),
    )
    rctx.file(
        "BUILD.bazel",
        'alias(name = "{}", actual = "{}", visibility = ["//visibility:public"])'.format(rctx.original_name, out),
    )

test2json = repository_rule(
    implementation = _test2json_impl,
    attrs = {"_go_sdk": attr.label(default = "@go_work_sdk//:ROOT")},
)
