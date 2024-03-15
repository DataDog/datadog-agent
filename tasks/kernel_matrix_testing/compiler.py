import sys

from tasks.kernel_matrix_testing.tool import info


def compiler_built(ctx):
    res = ctx.run("docker images kmt:compile | grep -v REPOSITORY | grep kmt", warn=True)
    return res.ok


def docker_exec(ctx, cmd, user="compiler", verbose=True, run_dir=None):
    if run_dir:
        cmd = f"cd {run_dir} && {cmd}"

    if not compiler_running(ctx):
        info("[*] Compiler not running, starting it...")
        start_compiler(ctx)

    ctx.run(f"docker exec -u {user} -i kmt-compiler bash -c \"{cmd}\"", hide=(not verbose))


def start_compiler(ctx):
    if not compiler_built(ctx):
        build_compiler(ctx)

    if compiler_running(ctx):
        ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")")

    ctx.run(
        "docker run -d --restart always --name kmt-compiler --mount type=bind,source=./,target=/datadog-agent kmt:compile sleep \"infinity\""
    )

    uid = ctx.run("id -u").stdout.rstrip()
    gid = ctx.run("id -g").stdout.rstrip()
    docker_exec(ctx, f"getent group {gid} || groupadd -f -g {gid} compiler", user="root")
    docker_exec(ctx, f"getent passwd {uid} || useradd -m -u {uid} -g {gid} compiler", user="root")

    if sys.platform != "darwin":  # No need to change permissions in MacOS
        docker_exec(ctx, f"chown {uid}:{gid} /datadog-agent && chown -R {uid}:{gid} /datadog-agent", user="root")

    docker_exec(ctx, "apt install sudo", user="root")
    docker_exec(ctx, "usermod -aG sudo compiler && echo 'compiler ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers", user="root")
    docker_exec(ctx, f"install -d -m 0777 -o {uid} -g {uid} /go", user="root")


def compiler_running(ctx):
    res = ctx.run("docker ps -aqf \"name=kmt-compiler\"")
    if res.ok:
        return res.stdout.rstrip() != ""
    return False


def build_compiler(ctx):
    ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")", warn=True, hide=True)
    ctx.run("docker image rm kmt:compile", warn=True, hide=True)

    docker_build_args = [
        # Specify platform with --platform, even if we're running in ARM we want x86_64 images
        # Important because some packages needed by that image are not available in arm builds of debian
        "--platform",
        "linux/amd64",
    ]

    # Add build arguments (such as go version) from go.env
    with open("../datadog-agent-buildimages/go.env", "r") as f:
        for line in f:
            docker_build_args += ["--build-arg", line.strip()]

    docker_build_args_s = " ".join(docker_build_args)
    ctx.run(
        f"cd ../datadog-agent-buildimages && docker build {docker_build_args_s} -f system-probe_x64/Dockerfile -t kmt:compile ."
    )
