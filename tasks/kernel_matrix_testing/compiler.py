def compiler_built(ctx):
    res = ctx.run("docker images kmt:compile | grep -v REPOSITORY | grep kmt", warn=True)
    return res.ok


def docker_exec(ctx, cmd, user="root"):
    ctx.run(f"docker exec -u {user} -i kmt-compiler bash -c \"{cmd}\"")


def start_compiler(ctx):
    if not compiler_built(ctx):
        build_compiler(ctx)

    if not compiler_running(ctx):
        ctx.run(
            "docker run -d --restart always --name kmt-compiler --mount type=bind,source=./,target=/datadog-agent kmt:compile sleep \"infinity\""
        )

    uid = ctx.run("getent passwd $USER | cut -d ':' -f 3").stdout.rstrip()
    gid = ctx.run("getent group $USER | cut -d ':' -f 3").stdout.rstrip()
    docker_exec(ctx, f"getent group {gid} || groupadd -f -g {gid} compiler")
    docker_exec(ctx, f"getent passwd {uid} || useradd -m -u {uid} -g {gid} compiler")
    docker_exec(ctx, f"chown {uid}:{gid} /datadog-agent && chown -R {uid}:{gid} /datadog-agent")
    docker_exec(ctx, "apt install sudo")
    docker_exec(ctx, "usermod -aG sudo compiler && echo 'compiler ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers")
    docker_exec(ctx, f"install -d -m 0777 -o {uid} -g {uid} /go")


def compiler_running(ctx):
    res = ctx.run("docker ps -aqf \"name=kmt-compiler\"")
    if res.ok:
        return res.stdout.rstrip() != ""
    return False

def build_compiler(ctx):
    ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")", warn=True, hide=True)
    ctx.run("docker image rm kmt:compile", warn=True, hide=True)
    ctx.run("cd ../datadog-agent-buildimages && docker build -f system-probe_x64/Dockerfile -t kmt:compile .")


