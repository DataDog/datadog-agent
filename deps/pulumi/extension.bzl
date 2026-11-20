"""Fetches the Pulumi CLI and plugins matching the SDKs in a `go.sum`, as the `@pulumi` hub:
- `@pulumi//:cli`, which `//internal/tools:pulumi` runs, is the CLI at the version of `github.com/pulumi/pulumi/sdk/v3`,
  as the CLI and the SDKs are released together under one version, and the SDK's own installer defaults to the SDK's
  version. Bumping the SDK bumps the CLI. `@pulumi//:bin/pulumi` is the CLI next to its bundled language hosts.
- `@pulumi//:install` installs the plugins of the SDKs, laid out as `pulumi plugin install` does.

Like `pulumi plugin install`, which asks each SDK which plugin it needs, this reads the npm package of each `pulumi-*`
SDK in `go_sum`, at the SDK's version, e.g. `github.com/pulumi/pulumi-eks/sdk/v4 v4.4.0` maps to
https://registry.npmjs.org/@pulumi/eks/4.4.0:
- its `pulumi` field (Pulumi's `PulumiPluginJSON`) tells whether it's a plugin (`resource`) and names it,
- its `repository.url` gives the GitHub release to fetch, checked against the sums the release publishes,
- its `dependencies` listed in `npm_dependencies` are plugins it loads itself, e.g. `@pulumi/kubernetes` for `eks`.
npm metadata and release sums are recorded as facts in `MODULE.bazel.lock`, so that they're fetched only once.
"""

load("@bazel_lib//lib:base64.bzl", "base64")
load("@bazel_lib//lib:repo_utils.bzl", "repo_utils")
load("@bazel_lib//lib:strings.bzl", "chr")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

def _fact(mctx, facts, url, fetch):
    facts[url] = mctx.facts[url] if url in mctx.facts else fetch(mctx, url)
    return facts[url]

def _package_url(package, version):
    return "https://registry.npmjs.org/{}/{}".format(package, version.removeprefix("v"))

def _package(mctx, url):
    if not mctx.download(url = url, output = "package.json", allow_fail = True).success:
        return {}
    package = json.decode(mctx.read("package.json"))
    if not package.get("pulumi", {}).get("resource"):
        return {}
    return {
        "name": package["pulumi"]["name"],
        "dependencies": {
            dependency: "v{}".format(version.removeprefix("^"))
            for dependency, version in package.get("dependencies", {}).items()
            if dependency.startswith("@pulumi") and dependency != "@pulumi/pulumi"
        },
        "url": package["repository"]["url"].removeprefix("git+").removesuffix(".git"),
    }

def _integrities(mctx, url):
    mctx.download(url = url, output = "checksums.txt")
    integrities = {}
    for line in mctx.read("checksums.txt").splitlines():
        checksum, _, filename = line.partition("  ")
        integrities[filename.removeprefix("./")] = "{}-{}".format(
            "sha1" if len(checksum) == 40 else "sha256",
            base64.encode("".join([chr(int(checksum[i:i + 2], 16)) for i in range(0, len(checksum), 2)])),
        )
    return integrities

def _plugin(mctx, package, version, plugins, facts):
    name = "resource-{}-{}".format(package["name"], version)
    if name in plugins:
        return
    plugins[name] = "@{}".format(name)
    base_url = "{}/releases/download/{}/".format(package["url"], version)
    filename = "pulumi-{}-{}-{}.tar.gz".format(name, repo_utils.os(mctx), repo_utils.arch(mctx))
    http_archive(
        name = name,
        build_file_content = """load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")

pkg_files(
    name = "{name}",
    srcs = ["pulumi-resource-{res}{ext}"],
    attributes = pkg_attributes(mode = "0755"),
    prefix = "{name}",
    visibility = ["//visibility:public"],
)
""".format(ext = ".exe" if repo_utils.is_windows(mctx) else "", name = name, res = package["name"]),
        integrity = _fact(
            mctx,
            facts,
            "{}{}_{}_checksums.txt".format(base_url, package["url"].rpartition("/")[2], version.removeprefix("v")),
            _integrities,
        )[filename],
        urls = ["{}{}".format(base_url, filename)],
    )

def _hub_impl(rctx):
    ext = ".exe" if repo_utils.is_windows(rctx) else ""
    rctx.download_and_extract(
        url = rctx.attr.cli_url,
        integrity = rctx.attr.cli_integrity,
        # The bundled `pulumi-language-*` hosts stay next to `pulumi`, where Pulumi looks them up.
        output = "bin",
        strip_prefix = "pulumi/bin" if ext else "pulumi",
    )
    rctx.file("BUILD.bazel", """load("@bazel_skylib//rules:native_binary.bzl", "native_binary")
load("@rules_pkg//pkg:install.bzl", "pkg_install")

native_binary(
    name = "cli",
    src = "bin/pulumi{ext}",
    out = "pulumi{ext}",
    # Use the language hosts released with this CLI rather than any other on the PATH, e.g. a CI image's.
    env = {{"PULUMI_IGNORE_AMBIENT_PLUGINS": "true"}},
    visibility = ["//visibility:public"],
)

pkg_install(
    name = "install",
    srcs = [
        "{plugins}",
    ],
    visibility = ["//visibility:public"],
)
""".format(ext = ext, plugins = '",\n        "'.join(rctx.attr.plugins)))

_hub = repository_rule(implementation = _hub_impl, attrs = {
    "cli_integrity": attr.string(mandatory = True),
    "cli_url": attr.string(mandatory = True),
    "plugins": attr.string_list(mandatory = True),
})

_CLI_SDK = "github.com/pulumi/pulumi/sdk/v3"

def _cli(mctx, versions, facts):
    """(url, integrity) of the CLI release for the host, at the only version in `versions`."""
    if len(versions) != 1:
        fail("expected one {} version in the go.sum files, got: {}".format(_CLI_SDK, sorted(versions)))
    version = versions.keys()[0]
    base_url = "https://github.com/pulumi/pulumi/releases/download/{}/".format(version)
    filename = "pulumi-{}-{}-{}.{}".format(
        version,
        repo_utils.os(mctx),
        {"amd64": "x64"}.get(repo_utils.arch(mctx), repo_utils.arch(mctx)),
        "zip" if repo_utils.is_windows(mctx) else "tar.gz",
    )
    integrities = _fact(
        mctx,
        facts,
        "{}pulumi-{}-checksums.txt".format(base_url, version.removeprefix("v")),
        _integrities,
    )
    return "{}{}".format(base_url, filename), integrities[filename]

def _impl(mctx):
    facts = {}
    plugins = {}
    cli_versions = {}
    for tag in [tag for module in mctx.modules for tag in module.tags.from_file]:
        for line in mctx.read(tag.go_sum).splitlines():
            go_module, version, _ = line.split(" ")
            if version.endswith("/go.mod"):
                continue
            if go_module == _CLI_SDK:
                cli_versions[version] = True
                continue
            path = go_module.split("/")
            if len(path) < 3 or not path[2].startswith("pulumi-"):
                continue
            package = _fact(
                mctx,
                facts,
                _package_url("@{}/{}".format(path[1], path[2].removeprefix("pulumi-").removesuffix("-sdk")), version),
                _package,
            )
            if not package:
                continue
            _plugin(mctx, package, version, plugins, facts)
            for dependency, dependency_version in package["dependencies"].items():
                if dependency in tag.npm_dependencies:
                    _plugin(
                        mctx,
                        _fact(mctx, facts, _package_url(dependency, dependency_version), _package),
                        dependency_version,
                        plugins,
                        facts,
                    )
    cli_url, cli_integrity = _cli(mctx, cli_versions, facts)
    name = "pulumi"
    _hub(name = name, cli_integrity = cli_integrity, cli_url = cli_url, plugins = sorted(plugins.values()))
    return mctx.extension_metadata(
        facts = facts,
        reproducible = True,
        root_module_direct_deps = [name],
        root_module_direct_dev_deps = [],
    )

pulumi = module_extension(
    implementation = _impl,
    tag_classes = {"from_file": tag_class(attrs = {
        "go_sum": attr.label(mandatory = True),
        "npm_dependencies": attr.string_list(),
    })},
)
