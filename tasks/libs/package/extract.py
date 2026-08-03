import os
import sys

from invoke.context import Context


def extract_deb(ctx: Context, deb_path: str, extract_path: str):
    os.makedirs(extract_path)
    if sys.platform == 'darwin':
        # bsdtar (macOS default tar, via libarchive) supports the ar format used by .deb files;
        # BSD ar does not support the System V ar format used by .deb files
        ctx.run(f"tar xf {deb_path} -C {extract_path}")
    else:
        # GNU ar supports .deb's System V ar format; --output sets the extract directory
        ctx.run(f"ar x {deb_path} --output {extract_path}")
    with ctx.cd(extract_path):
        ctx.run("tar xf data.tar.xz")
        ctx.run("rm data.tar.xz")


def extract_rpm(ctx: Context, rpm_path: str, extract_path: str):
    os.makedirs(extract_path)
    ctx.run(f"tar xf {rpm_path} -C {extract_path}")
