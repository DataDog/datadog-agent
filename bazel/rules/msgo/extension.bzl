"""Module extension registering the Microsoft build of Go (msgo) SDK and toolchain for the host platform, at the
version pinned by a go.work file's `go` directive.

https://github.com/microsoft/go/blob/microsoft/main/eng/doc/Downloads.md#downloading-a-specific-version describes
the URL scheme this extension builds: immutable go{X.Y.Z}-{revision} links for reproducibility, and the mutable
go{X.Y.Z} link used to detect when a newer revision is available.
"""

load("@rules_go//go/private:go_mod.bzl", "version_from_go_work")  # buildifier: disable=bzl-visibility
load("@rules_go//go/private:sdk.bzl", "detect_host_platform", "go_download_sdk_rule")  # buildifier: disable=bzl-visibility

def _toolchains_impl(rctx):
    goos, goarch = detect_host_platform(rctx)
    major, minor, patch = rctx.attr.version.split(".")
    rctx.file("BUILD.bazel", """load("@rules_go//go/private:go_toolchain.bzl", "declare_bazel_toolchains")

declare_bazel_toolchains(
    go_toolchain_repo = "@msgo",
    exec_goarch = "{goarch}",
    exec_goos = "{goos}",
    major = "{major}",
    minor = "{minor}",
    patch = "{patch}",
    prerelease = "",
    sdk_name = "msgo",
    sdk_type = "remote",
)
""".format(goos = goos, goarch = goarch, major = major, minor = minor, patch = patch))

_msgo_toolchains = repository_rule(
    implementation = _toolchains_impl,
    attrs = {"version": attr.string(mandatory = True)},
)

def _warn_if_outdated(module_ctx, version, revision):
    latest_assets_url = "https://aka.ms/golang/release/latest/go{}.assets.json".format(version)
    if not module_ctx.download(url = latest_assets_url, output = "latest_assets.json", allow_fail = True).success:
        # buildifier: disable=print
        print("WARNING: msgo: couldn't download {} to check for a newer revision.".format(latest_assets_url))
        return
    latest_revision = json.decode(module_ctx.read("latest_assets.json"))["version"].rpartition("-")[-1]
    if latest_revision != str(revision):
        # buildifier: disable=print
        print((
            "WARNING: msgo: {version}-{latest} supersedes the pinned {version}-{revision}, likely fixing issues: " +
            "consider bumping msgo.from_file's `msgo_revision` attr in deps/go.MODULE.bazel."
        ).format(revision = revision, version = version, latest = latest_revision))

def _fetch_sdks(module_ctx, tag):
    module_ctx.download(
        url = "https://github.com/microsoft/go/releases/download/v{}/assets.json".format(tag),
        output = "assets.json",
    )
    return {
        "{}_{}".format(arch["env"]["GOOS"], arch["env"]["GOARCH"]): [arch["url"], arch["sha256"]]
        for arch in json.decode(module_ctx.read("assets.json"))["arches"]
        if "env" in arch  # unset for non-platform archives, e.g. the source tarball
    }

def _impl(module_ctx):
    from_files = [tag for module in module_ctx.modules for tag in module.tags.from_file]
    if len(from_files) != 1:
        fail("msgo.from_file must be called exactly once, got {}".format(len(from_files)))

    version = version_from_go_work(module_ctx, from_files[0].go_work)
    revision = from_files[0].msgo_revision
    _warn_if_outdated(module_ctx, version, revision)
    tag = "{}-{}".format(version, revision)
    sdks = module_ctx.facts.get(tag) or _fetch_sdks(module_ctx, tag)
    goos, goarch = detect_host_platform(module_ctx)
    go_download_sdk_rule(
        name = "msgo",
        goos = goos,
        goarch = goarch,
        sdks = sdks,
        version = version,
        urls = ["{}"],  # `sdks` holds the manifest's full archive URLs
        experiments = ["ms_nocgo_opensslcrypto"],  # because Linux's OpenSSL backend otherwise needs cgo (Go < 1.27)
    )
    _msgo_toolchains(
        name = "msgo_toolchains",
        version = version,
    )
    return module_ctx.extension_metadata(reproducible = True, facts = {tag: sdks})

msgo = module_extension(
    arch_dependent = True,
    implementation = _impl,
    os_dependent = True,
    tag_classes = {
        "from_file": tag_class(attrs = {
            "go_work": attr.label(mandatory = True),
            "msgo_revision": attr.int(mandatory = True),
        }),
    },
)
