# Copyright 2016 The Bazel Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# WARNING:
# https://github.com/bazelbuild/bazel/issues/17713
# .bzl files in this package (tools/build_defs/repo) are evaluated
# in a Starlark environment without "@_builtins" injection, and must not refer
# to symbols associated with build/workspace .bzl files
"""MODULE wrapper for http_archive that uses our single source of truth for dependencies.

Usage:
    dd_http_archive = use_repo_rule("//deps:http.bzl", "dd_http_archive")

    dd_http_archive(
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
"""

load("@@//deps:all_deps.bzl", "ALL_DEPS")
load(
    "@@//third_party/bazel/tools/build_defs/repo:cache.bzl",
    "DEFAULT_CANONICAL_ID_ENV",
    "get_default_canonical_id",
)
load(
    "@@//third_party/bazel/tools/build_defs/repo:utils.bzl",
    "download_remote_files",
    "get_auth",
    "patch",
    "symlink_files",
    "update_attrs",
    "workspace_and_buildfile",
)

dd_http_archive_attrs = {
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

def _update_http_archive_integrity_attrs(ctx, attrs, info, integrity):
    integrity_override = {}

    # We don't need to override the integrity attribute if sha256 is already specified.
    if not info.sha256 and not ctx.attr.integrity:
        integrity_override["integrity"] = integrity.archive
    if ctx.attr.remote_module_file_urls and not ctx.attr.remote_module_file_integrity:
        integrity_override["remote_module_file_integrity"] = integrity.remote_module_file
    if ctx.attr.remote_file_integrity != integrity.remote_files:
        integrity_override["remote_file_integrity"] = integrity.remote_files
    if ctx.attr.remote_patches != integrity.remote_patches:
        integrity_override["remote_patches"] = integrity.remote_patches
    if not integrity_override:
        return ctx.repo_metadata(reproducible = True)
    return ctx.repo_metadata(attrs_for_reproducibility = update_attrs(ctx.attr, attrs.keys(), integrity_override))

def _dd_http_archive_impl(ctx):
    """Implementation of the http_archive rule."""
    if ctx.attr.build_file and ctx.attr.build_file_content:
        fail("Only one of build_file and build_file_content can be provided.")

    info = ALL_DEPS[ctx.original_name]
    download_info = ctx.download_and_extract(
        info.urls,
        ctx.attr.add_prefix,
        info.sha256,
        ctx.attr.type,
        info.strip_prefix,
        canonical_id = ctx.attr.canonical_id or get_default_canonical_id(ctx, info.urls),
        auth = get_auth(ctx, info.urls),
        integrity = ctx.attr.integrity,
    )
    workspace_and_buildfile(ctx)

    remote_files_info = download_remote_files(ctx)
    remote_patches_info = patch(ctx)
    symlink_files(ctx)

    # Download the module file after applying patches since modules may decide
    # to patch their packaged module and the patch may not apply to the file
    # checked in to the registry. This overrides the file if it exists.
    remote_module_file_integrity = ""
    if ctx.attr.remote_module_file_urls:
        remote_module_file_integrity = ctx.download(
            ctx.attr.remote_module_file_urls,
            "MODULE.bazel",
            auth = get_auth(ctx, ctx.attr.remote_module_file_urls),
            integrity = ctx.attr.remote_module_file_integrity,
        ).integrity

    integrity = struct(
        archive = download_info.integrity,
        remote_files = {path: info.integrity for path, info in remote_files_info.items()},
        remote_patches = {url: info.integrity for url, info in remote_patches_info.items()},
        remote_module_file = remote_module_file_integrity,
    )

    return _update_http_archive_integrity_attrs(ctx, dd_http_archive_attrs, info, integrity)

dd_http_archive = repository_rule(
    implementation = _dd_http_archive_impl,
    attrs = dd_http_archive_attrs,
    environ = [DEFAULT_CANONICAL_ID_ENV],
)
