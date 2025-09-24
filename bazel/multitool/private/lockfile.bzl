"Utilities for interacting with the multitool lockfile."

def _check(condition, message):
    "fails if condition is False and emits message"
    if not condition:
        fail(message)

def _check_version(os, binary_os):
    # require bazel 7.1 on windows. Only do this check for windows artifacts to avoid regressing anyone
    # skip version check on windows if we don't have a release version. We can't tell from a hash what features we have.
    if os == "windows" and binary_os == "windows" and native.bazel_version:
        version = native.bazel_version.split(".")
        if int(version[0]) > 7 or (int(version[0]) == 7 and int(version[1]) >= 1):
            pass
        else:
            fail("multitool: windows platform requires bazel 7.1+ to read artifacts; current bazel is " + native.bazel_version)

def _load(ctx, lockfiles):
    tools = {}
    for lockfile in lockfiles:
        # TODO: validate no conflicts from multiple hub declarations and/or
        #  fix toolchains to also declare their versions and enable consumers
        #  to use constraints to pick the right one.
        #  (this is also a very naive merge at the tool level)
        tools = tools | json.decode(ctx.read(lockfile))

    # a special key says this JSON document conforms to a schema
    tools.pop("$schema", None)

    # validation
    for tool_name, tool in tools.items():
        for binary in tool["binaries"]:
            _check(
                binary["os"] in ["linux", "macos", "windows"],
                "{tool_name}: Unknown os '{os}'".format(
                    tool_name = tool_name,
                    os = binary["os"],
                ),
            )
            _check(
                binary["cpu"] in ["x86_64", "arm64"],
                "{tool_name}: Unknown cpu '{cpu}'".format(
                    tool_name = tool_name,
                    cpu = binary["cpu"],
                ),
            )
            _check_version(ctx.os.name, binary["os"])

    return tools

def _sort_fn(tup):
    return tup[0]

def _sorted(tools):
    return sorted(tools.items(), key = _sort_fn)

lockfile = struct(
    load_defs = _load,
    sorted_defs = _sorted,
)
