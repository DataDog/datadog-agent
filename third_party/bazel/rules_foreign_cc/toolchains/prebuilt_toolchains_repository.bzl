"""A module defining a repository rule for housing toolchain definitions"""

_BUILD_FILE = """\
load("@rules_foreign_cc//toolchains/native_tools:native_tools_toolchain.bzl", "native_tool_toolchain")

{toolchains}
"""

_TOOLCHAIN = """\
toolchain(
    name = "{repo}_toolchain",
    exec_compatible_with = {exec_compatible_with},
    toolchain = "@{repo}//:{tool}_tool",
    toolchain_type = "@rules_foreign_cc//toolchains:{tool}_toolchain",
)
"""

_TOOLCHAIN_BZL = """\
def register_toolchains():
    native.register_toolchain(
{toolchains}
    )
"""

_WORKSPACE_FILE = """\
workspace(name = "{}")
"""

def _prebuilt_toolchains_repository_impl(repository_ctx):
    build_file = repository_ctx.path("BUILD.bazel")
    repository_ctx.file(build_file, _BUILD_FILE.format(
        toolchains = "\n".join([
            _TOOLCHAIN.format(
                repo = repo,
                tool = repository_ctx.attr.tool,
                exec_compatible_with = compat,
            )
            for repo, compat in repository_ctx.attr.repos.items()
        ]),
    ))

    bzl_file = repository_ctx.path("toolchains.bzl")
    repository_ctx.file(bzl_file, _TOOLCHAIN_BZL.format(
        toolchains = "\n".join([
            "    \"@{repo}//:{tool}_tool\",".format(
                repo = repo,
                tool = repository_ctx.attr.tool,
            )
            for repo in repository_ctx.attr.repos.keys()
        ]),
    ))

    workspace_file = repository_ctx.path("WORKSPACE.bazel")
    repository_ctx.file(workspace_file, _WORKSPACE_FILE.format(
        repository_ctx.name,
    ))

prebuilt_toolchains_repository = repository_rule(
    doc = "A repository rule which houses toolchain definitions",
    implementation = _prebuilt_toolchains_repository_impl,
    attrs = {
        "repos": attr.string_list_dict(
            doc = "A mapping of repository names to platform restrictions",
            mandatory = True,
        ),
        "tool": attr.string(
            doc = "The name of the tool the toolchains represent",
            mandatory = True,
            values = ["cmake", "ninja"],
        ),
    },
)
