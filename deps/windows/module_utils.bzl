"""MODULE wrapper for http_file that uses variables from release.json.

This loads 'dependencies' from release.json and uses that to substitute into url paths.

Usage:
  get_file_using_release_constants = use_repo_rule(":module_utils.bzl", "get_file_using_release_constants")

  get_file_using_release_constants(
      name = "windows_npm_driver",
      # Note that this is the name of the element holding the SHA, not the SHA itself.
      sha256 = "WINDOWS_DDNPM_SHASUM",
      target_filename = "DDPROCMON.msm",
      url = "https://s3.amazonaws.com/dd-windowsfilter/builds/{WINDOWS_DDNPM_PATH}/ddprocmoninstall-{WINDOWS_DDNPM_VERSION}.msm",
  )
"""

load("@bazel_tools//tools/build_defs/repo:cache.bzl", "DEFAULT_CANONICAL_ID_ENV", "get_default_canonical_id")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "get_auth")

get_file_using_release_constants_attrs = {
    "executable": attr.bool(
        doc = "If the downloaded file should be made executable.",
    ),
    "target_filename": attr.string(
        default = "downloaded",
        doc = "Name assigned to the downloaded file.",
    ),
    "sha256": attr.string(doc = "Name of variable in release_info.dependencies for SHA"),
    "url": attr.string(),
    "urls": attr.string_list(),
    "auth_patterns": attr.string_dict(),
    "canonical_id": attr.string(),
    "_release_info": attr.label(default = "//:release.json", allow_single_file = True),
}

def _get_source_urls(ctx, substitutions):
    """Returns source urls provided via the url, urls attributes.

    Also checks that at least one url is provided."""
    if not ctx.attr.url and not ctx.attr.urls:
        fail("At least one of url and urls must be provided")

    source_urls = []
    if ctx.attr.urls:
        source_urls = ctx.attr.urls
    if ctx.attr.url:
        source_urls = [ctx.attr.url] + source_urls
    return [url.format(**substitutions) for url in source_urls]

_GET_FILE_BUILD = """\
package(default_visibility = ["//visibility:public"])

exports_files(glob(["**"]))

filegroup(
    name = "file",
    srcs = ["{target_filename}"],
)
"""

def _get_file_using_release_constants_impl(rctx):
    """Implementation of the get_file_using_release_constants rule."""
    release_info = json.decode(rctx.read(rctx.path(rctx.attr._release_info)))
    vars = release_info["dependencies"]
    repo_root = rctx.path(".")
    forbidden_files = [
        repo_root,
        rctx.path("MODULE.bazel"),
        rctx.path("BUILD"),
    ]
    target_filename = rctx.attr.target_filename
    download_path = rctx.path("file/" + target_filename)
    if download_path in forbidden_files or not str(download_path).startswith(str(repo_root)):
        fail("'%s' cannot be used as target_filename in get_file_using_release_constants" % rctx.attr.target_filename)
    source_urls = _get_source_urls(rctx, vars)
    rctx.download(
        source_urls,
        target_filename,
        vars[rctx.attr.sha256],
        rctx.attr.executable,
        canonical_id = rctx.attr.canonical_id or get_default_canonical_id(rctx, source_urls),
        auth = get_auth(rctx, source_urls),
    )
    rctx.file("MODULE.bazel", "module(name = \"{name}\")\n".format(name = rctx.name))
    rctx.file("BUILD", _GET_FILE_BUILD.format(target_filename = target_filename))
    return rctx.repo_metadata(reproducible = True)

get_file_using_release_constants = repository_rule(
    implementation = _get_file_using_release_constants_impl,
    attrs = get_file_using_release_constants_attrs,
    environ = [DEFAULT_CANONICAL_ID_ENV],
)
