"""Discovers the host DotSlash executable without acquiring tool artifacts."""

load("@toml.bzl//:toml.bzl", "toml")

def _host_dotslash_impl(ctx):
    config = ctx.path(ctx.attr.config)
    version = toml.decode(ctx.read(config), default = {})
    for key in ("tools", "dotslash", "version"):
        version = version.get(key) if type(version) == "dict" else None
    if type(version) != "string" or not version.strip():
        version = ""
    dotslash = ctx.which("dotslash")
    error = "DotSlash is unavailable. Provide the version pinned in mise.toml on PATH; developer shells can use `mise exec -- bazel ...`."
    cache = ""
    if not version:
        error = "Invalid DotSlash pin in mise.toml. Set tools.dotslash.version to a nonempty version string in valid TOML."
    elif dotslash:
        ctx.watch(dotslash)
        result = ctx.execute([dotslash, "--version"], working_directory = str(config.dirname))
        error = "Expected DotSlash %s on PATH, as pinned in mise.toml. Update the host DotSlash executable; developer shells can use `mise exec -- bazel ...`.\n%s%s" % (version, result.stdout, result.stderr)
        if result.return_code == 0 and result.stdout.strip() == "DotSlash " + version:
            result = ctx.execute([dotslash, "--", "cache-dir"], working_directory = str(config.dirname))
            if result.return_code == 0:
                cache = result.stdout.strip()
                error = ""
            else:
                error = "Cannot locate the host DotSlash cache: " + result.stderr
    host = {
        "path": str(dotslash) if dotslash else "",
        "version": version,
        "error": error,
        "env": {"PATH": ctx.getenv("PATH", ""), "DOTSLASH_CACHE": cache},
    }
    ctx.file("host.json", json.encode_indent(host) + "\n")
    ctx.file("BUILD.bazel", """\
load("@platforms//host:constraints.bzl", "HOST_CONSTRAINTS")
load("@@//bazel/dotslash:toolchain.bzl", "host_dotslash_toolchain")

host_dotslash_toolchain(
    name = "runtime",
    dotslash = {dotslash},
    environment = {environment},
    error = {error},
    discovery = "host.json",
)

toolchain(
    name = "toolchain",
    toolchain = ":runtime",
    toolchain_type = "@@//bazel/dotslash:toolchain_type",
    exec_compatible_with = HOST_CONSTRAINTS,
    visibility = ["//visibility:public"],
)
""".format(
        dotslash = repr(host["path"]),
        environment = repr(host["env"]),
        error = repr(host["error"]),
    ))

host_dotslash = repository_rule(
    implementation = _host_dotslash_impl,
    local = True,
    environ = ["PATH", "DOTSLASH_CACHE", "XDG_CACHE_HOME", "HOME", "USERPROFILE"],
    attrs = {"config": attr.label(default = "//:mise.toml", allow_single_file = True)},
)
