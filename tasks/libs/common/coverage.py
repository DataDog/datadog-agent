import platform

from invoke import Context

from tasks.libs.common.utils import get_distro


def upload_codecov(ctx: Context, coverage_file: str, extra_tag: list[str]):
    distro_tag = get_distro()
    tags = [distro_tag]
    codecov_binary = "codecov" if platform.system() != "Windows" else "codecov.exe"

    if extra_tag:
        tags.extend(extra_tag)

    # Build the flags string with all tags
    flags_string = " ".join([f"-F {tag}" for tag in tags])
    ctx.run(f"{codecov_binary} --git-service=github -f {coverage_file} {flags_string}", warn=True, timeout=2 * 60)
