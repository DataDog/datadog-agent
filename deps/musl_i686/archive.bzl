"""http_archive wrapper that drops case-colliding terminfo (macOS APFS)."""

_EXTRACT_PY = """
import os
import shutil
import sys
import tarfile

src, dest, prefix = sys.argv[1], sys.argv[2], sys.argv[3]
skip_parts = ("/share/terminfo/", "/share/locale/", "/share/man/", "/share/doc/")
extract_kw = {}
if hasattr(tarfile, "data_filter"):
    extract_kw["filter"] = "data"
with tarfile.open(src) as tf:
    for member in tf.getmembers():
        path = "/" + member.name + "/"
        if any(part in path for part in skip_parts):
            continue
        tf.extract(member, dest, **extract_kw)
root = os.path.join(dest, prefix)
for name in os.listdir(root):
    shutil.move(os.path.join(root, name), os.path.join(dest, name))
os.rmdir(root)
"""

def _bootlin_musl_archive_impl(rctx):
    rctx.download(
        url = rctx.attr.urls,
        output = "tc.tar.xz",
        sha256 = rctx.attr.sha256,
    )
    rctx.file("extract.py", _EXTRACT_PY)
    res = rctx.execute([
        "python3",
        "extract.py",
        "tc.tar.xz",
        ".",
        rctx.attr.strip_prefix,
    ])
    if res.return_code != 0:
        fail(res.stderr or res.stdout)
    rctx.delete("tc.tar.xz")
    rctx.delete("extract.py")
    rctx.file("BUILD.bazel", rctx.read(rctx.attr.build_file))

bootlin_musl_archive = repository_rule(
    implementation = _bootlin_musl_archive_impl,
    attrs = {
        "build_file": attr.label(mandatory = True, allow_single_file = True),
        "sha256": attr.string(mandatory = True),
        "strip_prefix": attr.string(mandatory = True),
        "urls": attr.string_list(mandatory = True),
    },
)
