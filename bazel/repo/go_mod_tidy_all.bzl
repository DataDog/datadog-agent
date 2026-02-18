"""Repository rule generating one `go mod tidy` command per go.mod plus a global target to run them all in parallel as:
- bazel run @go_mod_tidy_all
- bazel run @go_mod_tidy_all -- -x
"""

load("@re.bzl", "re")

_COMMAND_TEMPLATE = """command(
    name = "go_mod_tidy_{name}",
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

def _parse_modules_yaml(content):
    modules = re.search(r"^modules:\r?\n((?:(?:[\t #][^\n]*)?\r?\n)+)", content, re.M)
    if not modules:
        fail("No modules found!")
    return {
        path: re.sub(r"\W", "_", path)
        for path, tag in re.findall(r"^  (\S+):\s*(\S*)", modules.group(1), re.M)
        if tag != "ignored"
    }

def _impl(repository_ctx):
    modules = _parse_modules_yaml(repository_ctx.read(repository_ctx.attr.modules_yml))
    repository_ctx.file("BUILD.bazel", _BUILD_TEMPLATE.format(
        command_defs = "\n".join([
            _COMMAND_TEMPLATE.format(name = name, go = repository_ctx.attr.go, path = path)
            for path, name in modules.items()
        ]),
        command_refs = "\n".join([
            '        ":go_mod_tidy_{name}",'.format(name = name)
            for name in modules.values()
        ]),
    ))

go_mod_tidy_all = repository_rule(
    attrs = {
        "go": attr.label(default = "@rules_go//go"),
        "modules_yml": attr.label(default = "//:modules.yml", allow_single_file = True),
    },
    implementation = _impl,
    local = True,
)
