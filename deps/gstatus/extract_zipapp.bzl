"""Repository rule extracting the gstatus zipapp, whose shebang line Bazel's zip extractor rejects."""

def _impl(rctx):
    content = rctx.read(rctx.path(rctx.attr.src))
    rctx.file("zipapp.zip", content[content.index("\n") + 1:], legacy_utf8 = False)
    rctx.extract("zipapp.zip", rename_files = {
        "glustercli-0.8.3.dist-info/COPYING-LGPLV3": "glustercli-COPYING-LGPLV3",
    })
    rctx.delete("zipapp.zip")
    rctx.file("BUILD.bazel", """
package(default_visibility = ["//visibility:public"])

exports_files(["glustercli-COPYING-LGPLV3"])

filegroup(name = "srcs", srcs = ["__main__.py", "version.py"] + glob(["gluster*/**/*.py"]))
""")

extract_zipapp = repository_rule(
    implementation = _impl,
    attrs = {
        "src": attr.label(allow_single_file = True, mandatory = True),
    },
)
