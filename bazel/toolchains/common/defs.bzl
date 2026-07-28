"""Utilitites for creating toolchains to wrap system provided tools."""

def _write_toolchain_repo(rctx, repo_name, tool_name, tool_path, tool_version = "<unknown>", exec_compatible_with = None):
    if not tool_path:
        tool_path = ""
    rctx.template(
        "BUILD",
        Label("@@//bazel/toolchains/common:toolchain_BUILD.tpl"),
        substitutions = {
            "{AVAILABLE}": "1" if tool_path else "0",
            "{EXEC_COMPATIBLE_WITH}": repr(exec_compatible_with),
            "{GENERATOR}": "//bazel/toolchains/common:defs.bzl",
            "{REPO_NAME}": repo_name,
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
            "{AVAILABLE}": "1" if tool_path else "0",
            "{GENERATOR}": "//bazel/toolchains/common:defs.bzl",
            "{REPO_NAME}": repo_name,
            "{TOOL_NAME}": tool_name,
        },
        executable = False,
    )

def _default_repo_builder_impl(rctx):
    tool_name = rctx.attr.tool_name
    tool_path = rctx.which(tool_name)
    if rctx.attr.verbose:
        if tool_path:
            print("Found %s at '%s'" % (tool_name, tool_path))  # buildifier: disable=print
        else:
            print("No system %s found." % tool_name)  # buildifier: disable=print
    _write_toolchain_repo(
        rctx = rctx,
        repo_name = rctx.original_name,
        tool_name = rctx.attr.tool_name,
        tool_path = tool_path,
        exec_compatible_with = rctx.attr.exec_compatible_with,
    )

def make_repo_builder(name, tool_name, impl = _default_repo_builder_impl):
    return repository_rule(
        implementation = impl,
        doc = """Create a repository that defines a {name} toolchain based on tool in the default $PATH.""".format(name = name),
        local = True,
        environ = ["PATH"],
        attrs = {
            "tool_name": attr.string(doc = "The name of the tool to find.", default = tool_name),
            "exec_compatible_with": attr.string_list(
                doc = "exec_compatible_with list to apply to the created toolchain.",
            ),
            "verbose": attr.bool(
                doc = "If true, print status messages.",
            ),
        },
    )
