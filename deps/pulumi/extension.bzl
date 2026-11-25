"""Fetches the Pulumi CLI and plugins matching the SDKs in a `go.sum`, as the `@pulumi` hub:
- `bazel run @pulumi <args>` runs the CLI from the current directory, with the hermetic Go SDK first on the `PATH`,
- `bazel run @pulumi//:install -- --destdir=<Pulumi home>` installs the CLI next to its bundled Go language host in
  `bin`, where the Automation API looks for it, and the plugins in `plugins`, laid out as `pulumi plugin install` does.

The CLI is the GitHub release at the version of `github.com/pulumi/pulumi/sdk`, checked against the sums it publishes,
as the CLI and the SDKs are released together under one version, and the SDK's own installer defaults to the SDK's.

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

def _plugin(mctx, package, version):
    name = "resource-{}-{}".format(package["name"], version)
    return {name: struct(
        archive = "pulumi-{}-{}-{}.tar.gz".format(name, repo_utils.os(mctx), repo_utils.arch(mctx)),
        checksums = "{}_{}_checksums.txt".format(package["url"].rpartition("/")[2], version.removeprefix("v")),
        files = ["pulumi-resource-{}".format(package["name"])],
        prefix = "plugins/{}".format(name),
        repo_url = package["url"],
        strip_prefix = "",
        version = version,
    )}

def _hub_impl(rctx):
    rctx.file("BUILD.bazel", """load("@bazel_skylib//rules:native_binary.bzl", "native_binary")
load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@@//bazel/rules/go_shim:defs.bzl", "go_shim")

native_binary(
    name = "cli",
    src = "@pulumi_cli//:pulumi{ext}",
    out = "pulumi{ext}",
    env = {{
        "PULUMI_IGNORE_AMBIENT_PLUGINS": "true",
        "PULUMI_SKIP_UPDATE_CHECK": "true",
    }},
)

go_shim(
    name = "pulumi",
    tool = ":cli",
    visibility = ["//visibility:public"],
)

pkg_install(
    name = "install",
    srcs = [
        "{assets}",
    ],
    visibility = ["//visibility:public"],
)
""".format(assets = '",\n        "'.join(rctx.attr.assets), ext = rctx.attr.ext))

_hub = repository_rule(implementation = _hub_impl, attrs = {
    "assets": attr.string_list(mandatory = True),
    "ext": attr.string(mandatory = True),
})

def _impl(mctx):
    facts = {}
    assets = {}
    for tag in [tag for module in mctx.modules for tag in module.tags.from_file]:
        for line in mctx.read(tag.go_sum).splitlines():
            go_module, version, _ = line.split(" ")
            if version.endswith("/go.mod"):
                continue
            path = go_module.split("/")
            if path[2:4] == ["pulumi", "sdk"]:
                assets["pulumi_cli"] = struct(
                    archive = "pulumi-{}-{}-{}.{}".format(
                        version,
                        repo_utils.os(mctx),
                        repo_utils.arch(mctx).replace("amd64", "x64"),
                        "zip" if repo_utils.is_windows(mctx) else "tar.gz",
                    ),
                    checksums = "pulumi-{}-checksums.txt".format(version.removeprefix("v")),
                    files = ["pulumi", "pulumi-language-go"],
                    prefix = "bin",
                    repo_url = "https://{}".format("/".join(path[:3])),
                    strip_prefix = "pulumi/bin" if repo_utils.is_windows(mctx) else "pulumi",
                    version = version,
                )
                continue
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
            assets |= _plugin(mctx, package, version)
            for dependency, dependency_version in package["dependencies"].items():
                if dependency in tag.npm_dependencies:
                    assets |= _plugin(
                        mctx,
                        _fact(mctx, facts, _package_url(dependency, dependency_version), _package),
                        dependency_version,
                    )
    ext = ".exe" if repo_utils.is_windows(mctx) else ""
    for name, asset in assets.items():
        release_url = "{}/releases/download/{}/".format(asset.repo_url, asset.version)
        http_archive(
            name = name,
            build_file_content = """load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")

exports_files([{srcs}])

pkg_files(
    name = "{name}",
    attributes = pkg_attributes(mode = "0755"),
    prefix = "{prefix}",
    srcs = [{srcs}],
    visibility = ["//visibility:public"],
)
""".format(name = name, prefix = asset.prefix, srcs = ", ".join(['"{}{}"'.format(base, ext) for base in asset.files])),
            integrity = _fact(mctx, facts, "{}{}".format(release_url, asset.checksums), _integrities)[asset.archive],
            strip_prefix = asset.strip_prefix,
            urls = ["{}{}".format(release_url, asset.archive)],
        )
    name = "pulumi"
    _hub(name = name, assets = ["@{}".format(name) for name in sorted(assets)], ext = ext)
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
