"""Build Wine's message compiler from the Wine 11.20 sources.

rules_wine ships a runtime built with `make install-lib`, which does not
install wmc. The NLS codepage files are already in that source tarball, and
wmc reads them through `--nls-dir`, so the compiler and the tables stay on
the same Wine version.
"""

def _wine_wmc_repository_impl(rctx):
    rctx.download_and_extract(
        sha256 = rctx.attr.sha256,
        stripPrefix = rctx.attr.strip_prefix,
        type = "tar.gz",
        url = rctx.attr.urls,
    )

    # configure output is not in the source tarball. This is the slice wmc includes.
    # tools/wmc/mcy.y is compiled by bison_cc_library in the overlay BUILD.
    rctx.template("include/config.h", rctx.attr.config_h, substitutions = {})
    rctx.symlink(rctx.attr.build_file, "BUILD.bazel")

wine_wmc_repository = repository_rule(
    attrs = {
        "build_file": attr.label(
            allow_single_file = True,
            mandatory = True,
        ),
        "config_h": attr.label(
            allow_single_file = True,
            mandatory = True,
        ),
        "sha256": attr.string(mandatory = True),
        "strip_prefix": attr.string(mandatory = True),
        "urls": attr.string_list(mandatory = True),
    },
    implementation = _wine_wmc_repository_impl,
)
