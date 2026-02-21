"""Repository rule generating one `go mod tidy` command per go.mod plus a global target to run them all in parallel as:
- bazel run @go_mod_tidy_all
- bazel run @go_mod_tidy_all -- -x
"""

load("@re.bzl", "re")
load("//bazel/repo:parse_go_work.bzl", "parse_go_work")

_COMMAND_TEMPLATE = """command(
    name = "{name}",
    arguments = [
        "-C",
        "{path}",
        "mod",
        "tidy",
    ],
    command = "{go}",
    description = "go -C {path} mod tidy",
    run_from_workspace_root = True,
)
"""

_BUILD_TEMPLATE = """load("@rules_multirun//:defs.bzl", "command", "multirun")

{command_defs}
multirun(
    name = "go_mod_tidy_all",
    buffer_output = True,  # unravel
    commands = [
{command_refs}
    ],
    jobs = 0,  # parallelize
)
"""

def _impl(rctx):
    modules = {
        "go_mod_tidy_{}".format(re.sub(r"\W", "_", path)): path
        for path in parse_go_work(rctx).paths
    }
    rctx.file("BUILD.bazel", _BUILD_TEMPLATE.format(
        command_defs = "\n".join([
            _COMMAND_TEMPLATE.format(name = name, go = rctx.attr.go, path = path)
            for name, path in modules.items()
        ]),
        command_refs = "\n".join([
            '        ":{name}",'.format(name = name)
            for name in modules
        ]),
    ))

go_mod_tidy_all = repository_rule(
    attrs = {
        "go": attr.label(default = "@rules_go//go"),
        "go_work": attr.label(default = "//:go.work", allow_single_file = True),
    },
    implementation = _impl,
    local = True,
)
