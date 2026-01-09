"""Module extension wrapper for http_archive that uses our single source of truth for dependencies.

Usage:
    dd_http = use_extension("//deps:dd_http.bzl", "agent_deps")

    dd_http.archive(
        name = "xz",
        build_file = "//deps:xz.BUILD.bazel",
        files = {
            "config.lzma-linux-arm64.h": "//deps/xz:config.lzma-linux-arm64.h",
            "config.lzma-linux-x86_64.h": "//deps/xz:config.lzma-linux-x86_64.h",
            "config.lzma-osx-arm64.h": "//deps/xz:config.lzma-osx-arm64.h",
            "config.lzma-osx-x86_64.h": "//deps/xz:config.lzma-osx-x86_64.h",
            "config.lzma-windows.h": "//deps/xz:config.lzma-windows.h",
        },
    )
    use_repo(dd_http, "xz")
"""

load("@@//deps:all_deps.bzl", "ALL_DEPS")
load("@@//third_party/bazel/tools/build_defs/repo:http.bzl", "http_archive")

_archive_attrs = {
    "name": attr.string(
        default = "",
        doc = "The name used for both lookup on centralized deps dictionary and as a name for the repo.",
        mandatory = True,
    ),
    # The attributes below are possible attributes to pass over to http_archive
    "integrity": attr.string(),
    "netrc": attr.string(),
    "auth_patterns": attr.string_dict(),
    "canonical_id": attr.string(),
    "add_prefix": attr.string(default = ""),
    "files": attr.string_keyed_label_dict(),
    "type": attr.string(),
    "patches": attr.label_list(default = []),
    "remote_file_urls": attr.string_list_dict(default = {}),
    "remote_file_integrity": attr.string_dict(default = {}),
    "remote_module_file_urls": attr.string_list(default = []),
    "remote_module_file_integrity": attr.string(default = ""),
    "remote_patches": attr.string_dict(default = {}),
    "remote_patch_strip": attr.int(default = 0),
    "patch_tool": attr.string(default = ""),
    "patch_args": attr.string_list(default = []),
    "patch_strip": attr.int(default = 0),
    "patch_cmds": attr.string_list(default = []),
    "patch_cmds_win": attr.string_list(default = []),
    "build_file": attr.label(allow_single_file = True),
    "build_file_content": attr.string(),
    "workspace_file": attr.label(),
    "workspace_file_content": attr.string(),
}

def _dd_http_impl(ctx):
    for mod in ctx.modules:
        for repo_tag in mod.tags.archive:
            # Extract input attributes into a dict
            kwargs = {key: getattr(repo_tag, key) for key in dir(repo_tag)}
            # Take info from the centralized source of truth to populate additional fields
            dep_info = ALL_DEPS[repo_tag.name]
            kwargs["sha256"] = dep_info.sha256
            kwargs["strip_prefix"] = dep_info.strip_prefix
            kwargs["urls"] = dep_info.urls
            # Pass on the modified set of attributes to the http_archive rule
            http_archive(**kwargs)

dd_http = module_extension(
    implementation = _dd_http_impl,
    tag_classes = {"archive": tag_class(_archive_attrs)},
)
