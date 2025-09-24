"multitool templating"

load("@platforms//host:constraints.bzl", "HOST_CONSTRAINTS")

_HUB_TEMPLATE = "//bazel/multitool/private:hub_repo_template/{filename}.template"
_HUB_TOOL_TEMPLATE = "//bazel/multitool/private:hub_repo_tool_template/{filename}.template"

_TOOL_TEMPLATE = "//bazel/multitool/private:tool_repo_template/{filename}.template"
_TOOL_TOOL_TEMPLATE = "//bazel/multitool/private:tool_repo_tool_template/{filename}.template"

# map from HOST_CONSTRAINTS to supported_os and supported_cpu from lockfile.schema.json
_HOST_CONSTRAINTS_MAPPING = {
    "@platforms//cpu:aarch64": "arm64",
    "@platforms//cpu:x86_64": "x86_64",
    "@platforms//os:osx": "macos",
    "@platforms//os:linux": "linux",
    "@platforms//os:windows": "windows",
}

def _render_tool(rctx, filename, substitutions = None):
    rctx.template(
        filename,
        Label(_TOOL_TEMPLATE.format(filename = filename)),
        substitutions = substitutions or {},
    )

def _render_tool_tool(rctx, tool_name, filename, substitutions = None):
    rctx.template(
        "tools/{tool_name}/{filename}".format(tool_name = tool_name, filename = filename),
        Label(_TOOL_TOOL_TEMPLATE.format(filename = filename)),
        substitutions = {
            "{name}": tool_name,
        } | (substitutions or {}),
    )

def _render_hub(rctx, filename, substitutions = None):
    rctx.template(
        filename,
        Label(_HUB_TEMPLATE.format(filename = filename)),
        substitutions = substitutions or {},
    )

def _render_hub_tool(rctx, tool_name, filename, substitutions = None):
    rctx.template(
        "tools/{tool_name}/{filename}".format(tool_name = tool_name, filename = filename),
        Label(_HUB_TOOL_TEMPLATE.format(filename = filename)),
        substitutions = {
            "{name}": tool_name,
        } | (substitutions or {}),
    )

def _render_tool_labels(tools):
    supported_host_constraints = [_HOST_CONSTRAINTS_MAPPING.get(constraint, None) for constraint in HOST_CONSTRAINTS]
    host_tool_keys = [
        tool_key
        for tool_key, tool in tools.items()
        if any([binary["os"] in supported_host_constraints and binary["cpu"] in supported_host_constraints for binary in tool["binaries"]])
    ]

    return "\n".join([
        "    \"{tool_name}\": Label(\"//tools/{tool_name}\"),".format(tool_name = tool_name)
        for tool_name in host_tool_keys
    ])

def _render_tool_repo(hub_name, tool_name, binary):
    name = "{name}.{tool_name}.{os}_{cpu}".format(
        name = hub_name,
        tool_name = tool_name,
        os = binary["os"],
        cpu = binary["cpu"],
    )
    return "\n".join([
        "    tool_repo(",
        "        name = \"{name}\",".format(name = name),
        "        tool_name = \"{tool_name}\",".format(tool_name = tool_name),
        "        binary = '{binary}',".format(binary = json.encode(binary)),
        "    )",
        "",
    ])

def _render_tool_repos(hub_name, tools):
    if len(tools) == 0:
        # in the case the tool dict is empty, ensure the function body we're
        # templating into is non-empty
        return "    pass\n"

    return "\n".join([
        _render_tool_repo(hub_name, tool_name, binary)
        for tool_name, tool in tools.items()
        for binary in tool["binaries"]
    ])

def _tools_subs(hub_name, tools):
    return {
        "{tool_labels}": _render_tool_labels(tools),
        "{tool_repos}": _render_tool_repos(hub_name, tools),
    }

templates = struct(
    hub = _render_hub,
    hub_tool = _render_hub_tool,
    tool = _render_tool,
    tool_tool = _render_tool_tool,
    tools_substitutions = _tools_subs,
)
