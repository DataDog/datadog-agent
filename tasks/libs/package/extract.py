import os

from invoke.context import Context


def extract_deb(ctx: Context, deb_path: str, extract_path: str):
    os.makedirs(extract_path)
    ctx.run(f"tar xf {deb_path} -C {extract_path}")
    with ctx.cd(extract_path):
        ctx.run("tar xf data.tar.xz")
        ctx.run("rm data.tar.xz")


def extract_rpm(ctx: Context, rpm_path: str, extract_path: str):
    os.makedirs(extract_path)
    ctx.run(f"tar xf {rpm_path} -C {extract_path}")
