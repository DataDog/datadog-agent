"""Repository rule that exposes the data payload of a pinned .deb."""

def _deb_archive_impl(rctx):
    # type = "deb" only unpacks the outer ar archive; the files live in data.tar.xz.
    rctx.download_and_extract(url = rctx.attr.urls, sha256 = rctx.attr.sha256, type = "deb", output = "_deb")
    rctx.extract("_deb/data.tar.xz")
    rctx.delete("_deb")
    rctx.symlink(rctx.attr.build_file, "BUILD.bazel")

deb_archive = repository_rule(
    implementation = _deb_archive_impl,
    attrs = {
        "build_file": attr.label(mandatory = True, allow_single_file = True),
        "sha256": attr.string(mandatory = True),
        "urls": attr.string_list(mandatory = True),
    },
)
