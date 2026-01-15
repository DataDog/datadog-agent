"""MODULE wrapper for http_archive that uses our single source of truth for dependencies.

Usage:
    dd_http = use_extension("//deps:dd_http.bzl", "dd_http")

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

dd_http_archive_attrs = {
    "add_prefix": attr.string(default = ""),
    "auth_patterns": attr.string_dict(),
    "build_file_content": attr.string(),
    "build_file": attr.label(allow_single_file = True),
    "canonical_id": attr.string(),
    "files": attr.string_keyed_label_dict(),
    "integrity": attr.string(),
    "name": attr.string(),
    "netrc": attr.string(),
    "patch_args": attr.string_list(default = []),
    "patch_cmds_win": attr.string_list(default = []),
    "patch_cmds": attr.string_list(default = []),
    "patch_strip": attr.int(default = 0),
    "patch_tool": attr.string(default = ""),
    "patches": attr.label_list(default = []),
    "remote_file_integrity": attr.string_dict(default = {}),
    "remote_file_urls": attr.string_list_dict(default = {}),
    "remote_module_file_integrity": attr.string(default = ""),
    "remote_module_file_urls": attr.string_list(default = []),
    "remote_patch_strip": attr.int(default = 0),
    "remote_patches": attr.string_dict(default = {}),
    "type": attr.string(),
    "workspace_file_content": attr.string(),
    "workspace_file": attr.label(),
}

def _dd_http_impl(ctx):
    for mod in ctx.modules:
        for repo in mod.tags.archive:
            kwargs = {key: getattr(repo, key) for key in dir(repo) if getattr(repo, key)}
            dep = ALL_DEPS[repo.name]
            kwargs["sha256"] = dep.sha256
            kwargs["strip_prefix"] = dep.strip_prefix
            kwargs["urls"] = dep.urls
            http_archive(**kwargs)

dd_http = module_extension(
    implementation = _dd_http_impl,
    tag_classes = {
        "archive": tag_class(dd_http_archive_attrs),
    },
)
