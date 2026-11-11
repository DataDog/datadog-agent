"""Repo rule that unpacks a Debian package, like `http_archive` for debs.

A `.deb` is an `ar` archive whose payload (`data.tar.*`) is a nested tar archive.
Bazel's built-in decompressors handle both layers (`type = "ar"` for the outer
archive, `repository_ctx.extract` for the payload tar), so no host tools are
needed and fetching works on every platform Bazel runs on.

Since a deb payload never contains BUILD files, one of `build_file` or
`build_file_content` is required to expose targets, with `http_archive`'s
semantics: the BUILD file becomes the root of a single package, and files are
referenced by their paths relative to the repository root.

Usage:

    deb_archive = use_repo_rule("//bazel/repo:deb_archive.bzl", "deb_archive")

    deb_archive(
        name = "my_pkg_deb",
        urls = ["https://example.com/my_pkg_1.2.3_amd64.deb"],
        sha256 = "...",
        build_file = "//deps:my_pkg.BUILD.bazel",  # or build_file_content
    )
"""

def _deb_archive(rctx):
    """Implementation of the deb_archive rule."""

    if rctx.attr.build_file and rctx.attr.build_file_content:
        fail("Only one of build_file and build_file_content can be provided.")

    # Unpack the outer `ar` archive into a staging directory, so its members
    # (debian-binary, control.tar.*, data.tar.*, signatures, ...) can be removed
    # wholesale once the payload is extracted — no need to enumerate their names.
    rctx.download_and_extract(
        rctx.attr.urls,
        output = ".deb",
        sha256 = rctx.attr.sha256,
        type = "ar",
    )

    data_tar = None
    for member in sorted([entry.basename for entry in rctx.path(".deb").readdir() if not entry.is_dir]):
        if member.startswith("data.tar"):
            data_tar = member
            break
    if not data_tar:
        fail("the deb does not contain a data.tar member: %s" % rctx.attr.urls)

    # Unpack the payload with its original paths.
    rctx.report_progress("extracting %s" % data_tar)
    rctx.extract(".deb/" + data_tar)

    # The deb members are no longer needed; keep the repository minimal.
    rctx.delete(".deb")

    if rctx.attr.build_file:
        rctx.file("BUILD.bazel", rctx.read(rctx.attr.build_file))
    else:
        rctx.file("BUILD.bazel", rctx.attr.build_file_content)

deb_archive = repository_rule(
    implementation = _deb_archive,
    attrs = {
        "build_file": attr.label(
            allow_single_file = True,
            doc = "The file to use as the BUILD file for this repository, like http_archive's build_file.",
        ),
        "build_file_content": attr.string(
            doc = "The content of the BUILD file for this repository, like http_archive's build_file_content.",
        ),
        "sha256": attr.string(
            doc = "The SHA-256 checksum of the deb.",
            mandatory = True,
        ),
        "urls": attr.string_list(
            doc = "URLs of the deb, in priority order.",
            mandatory = True,
        ),
    },
    doc = "Unpacks a Debian package like http_archive, with deb-specific archive handling.",
)
