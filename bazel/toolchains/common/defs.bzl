"""Utilitites for creating toolchains to wrap system provided tools."""

def write_toolchain_repo(rctx, tool_name, tool_path, tool_version = "<unknown>"):
    if not tool_path:
        tool_path = ""
    rctx.template(
        "BUILD",
        Label("@@//bazel/toolchains/common:toolchain_BUILD.tpl"),
        substitutions = {
            "{GENERATOR}": "@@//bazel/toolchains/{tool_name}/{tool_name}_configure.bzl%find_system_{tool_name}".format(
                tool_name = tool_name,
            ),
            "{TOOL_NAME}": tool_name,
            "{TOOL_PATH}": str(tool_path),
            "{TOOL_VERSION}": tool_version,
        },
        executable = False,
    )
    rctx.template(
        "defs.bzl",
        Label("@@//bazel/toolchains/common:toolchain_defs.bzl.tpl"),
        substitutions = {
            "{GENERATOR}": "@@//bazel/toolchains/{tool_name}/{tool_name}_configure.bzl%find_system_{tool_name}".format(
                tool_name = tool_name,
            ),
            "{TOOL_NAME}": tool_name,
        },
        executable = False,
    )

def make_repo_builder(name, impl):
    return repository_rule(
        implementation = impl,
        doc = """Create a repository that defines an {name} toolchain based on the system {name}.""".format(name = name),
        local = True,
        environ = ["PATH"],
        attrs = {
            "verbose": attr.bool(
                doc = "If true, print status messages.",
            ),
        },
    )
