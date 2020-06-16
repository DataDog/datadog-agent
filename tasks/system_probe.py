import datetime
import glob
import os
import getpass
import contextlib
import shutil
import tempfile

from invoke import task
from subprocess import check_output, CalledProcessError

from .utils import bin_name, get_build_flags, REPO_PATH, get_version, get_git_branch_name, get_go_version, get_git_commit
from .build_tags import get_default_build_tags

BIN_DIR = os.path.join(".", "bin", "system-probe")
BIN_PATH = os.path.join(BIN_DIR, bin_name("system-probe", android=False))

EBPF_BUILDER_IMAGE = 'datadog/tracer-bpf-builder'
EBPF_BUILDER_FILE = os.path.join(".", "tools", "ebpf", "Dockerfiles", "Dockerfile-ebpf")

BPF_TAG = "linux_bpf"
BCC_TAG = "bcc"
GIMME_ENV_VARS = ['GOROOT', 'PATH']


@task
def build(ctx, race=False, go_version=None, incremental_build=False, major_version='7',
          python_runtimes='3', with_bcc=True, go_mod="vendor", windows=False):
    """
    Build the system_probe
    """

    # Only build ebpf files on unix
    if not windows:
        build_object_files(ctx, install=True)

    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx, major_version=major_version),
        "GoVersion": get_go_version(),
        "GitBranch": get_git_branch_name(),
        "GitCommit": get_git_commit(),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    goenv = {}
    if go_version:
        lines = ctx.run("gimme {version}".format(version=go_version)).stdout.split("\n")
        for line in lines:
            for env_var in GIMME_ENV_VARS:
                if env_var in line:
                    goenv[env_var] = line[line.find(env_var)+len(env_var)+1:-1].strip('\'\"')
        ld_vars["GoVersion"] = go_version

    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, python_runtimes=python_runtimes)

    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]
    env.update(goenv)

    # Add custom ld flags
    ldflags += ' '.join(["-X '{name}={value}'".format(name=main+key, value=value) for key, value in ld_vars.items()])

    if not windows:
        build_tags = get_default_build_tags() + [BPF_TAG]
    else:
        build_tags = get_default_build_tags()

    if with_bcc:
        build_tags.append(BCC_TAG)

    # TODO static option
    cmd = 'go build -mod={go_mod} {race_opt} {build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/system-probe'

    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "" if incremental_build else "-a",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": BIN_PATH,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)


@task
def build_in_docker(ctx, rebuild_ebpf_builder=False, race=False, incremental_build=False, major_version='7'):
    """
    Build the system_probe using a container
    This can be used when the current OS don't have up to date linux headers
    """

    if rebuild_ebpf_builder:
        build_ebpf_builder(ctx)

    docker_cmd = "docker run --rm \
            -v {cwd}:/go/src/github.com/DataDog/datadog-agent \
            --workdir=/go/src/github.com/DataDog/datadog-agent \
            {builder} \
            {cmd}"

    if should_use_sudo(ctx):
        docker_cmd = "sudo " + docker_cmd

    cmd = "invoke -e system-probe.build --major-version {}".format(major_version)

    if race:
        cmd += " --race"
    if incremental_build:
        cmd += " --incremental-build"

    ctx.run(docker_cmd.format(cwd=os.getcwd(), builder=EBPF_BUILDER_IMAGE, cmd=cmd))


@task
def test(ctx, skip_object_files=False, only_check_bpf_bytes=False):
    """
    Run tests on eBPF parts
    If skip_object_files is set to True, this won't rebuild object files
    If only_check_bpf_bytes is set to True this will only check that the assets bundled are
    matching the currently generated object files
    """

    if not skip_object_files:
        build_object_files(ctx, install=False)

    pkg = "./pkg/ebpf/... ./pkg/network/..."

    # Pass along the PATH env variable to retrieve the go binary path
    path = os.environ['PATH']

    cmd = 'go test -mod={go_mod} -v -tags "{bpf_tag}" {pkg}'
    if not is_root():
        cmd = 'sudo -E PATH={path} ' + cmd

    if only_check_bpf_bytes:
        cmd += " -run=TestEbpfBytesCorrect"
    else:
        if getpass.getuser()  != "root":
            print("system-probe tests must be run as root")
            raise Exit(code=1)
        if os.getenv("GOPATH") is None:
            print("GOPATH is not set, if you are running tests with sudo, you may need to use the -E option to preserve your environment")
            raise Exit(code=1)

    ctx.run(cmd.format(path=path, go_mod="vendor", bpf_tag=BPF_TAG, pkg=pkg))


@task
def nettop(ctx, incremental_build=False):
    """
    Build and run the `nettop` utility for testing
    """
    build_object_files(ctx, install=True)

    cmd = 'go build -mod={go_mod} {build_type} -tags "linux_bpf" -o {bin_path} {path}'
    bin_path = os.path.join(BIN_DIR, "nettop")
    # Build
    ctx.run(cmd.format(
        path=os.path.join(REPO_PATH, "pkg", "ebpf", "nettop"),
        bin_path=bin_path,
        go_mod=go_mod,
        build_type="" if incremental_build else "-a",
    ))

    # Run
    if should_use_sudo(ctx):
        ctx.sudo(bin_path)
    else:
        ctx.run(bin_path)


@task
def cfmt(ctx):
    """
    Format C code using clang-format
    """

    fmtCmd = "clang-format -i -style='{{BasedOnStyle: WebKit, BreakBeforeBraces: Attach}}' {file}"
    # This only works with gnu sed
    sedCmd = r"sed -i 's/__attribute__((always_inline)) /__attribute__((always_inline))\
/g' {file}"

    files = glob.glob("pkg/ebpf/c/*.[c,h]")

    for file in files:
        ctx.run(fmtCmd.format(file=file))
        ctx.run(sedCmd.format(file=file))


@task
def build_dev_docker_image(ctx, image_name, push=False):
    """
    Build a system-probe-agent Docker image (development only)
    if push is set to true the image will be pushed to the given registry
    """

    dev_file = os.path.join(".", "tools", "ebpf", "Dockerfiles", "Dockerfile-tracer-dev")
    cmd = "docker build {directory} -t {image_name} -f {file}"
    push_cmd = "docker push {image_name}"

    # Build in a temporary directory to make the docker build context small
    with tempdir() as d:
        shutil.copy(BIN_PATH, d)
        ctx.run(cmd.format(directory=d, image_name=image_name, file=dev_file))
        if push:
            ctx.run(push_cmd.format(image_name=image_name))


@task
def object_files(ctx, install=True):
    """object_files builds the eBPF object files"""
    build_object_files(ctx, install=install)


def build_object_files(ctx, install=True):
    """build_object_files builds only the eBPF object
    set install to False to disable replacing the assets
    """

    # if clang is missing, subsequent calls to ctx.run("clang ...") will fail silently, and result in us not building a
    # new .o file
    print("checking for clang executable...")
    ctx.run("which clang")
    print("found clang")

    centos_headers_dir = "/usr/src/kernels"
    debian_headers_dir = "/usr/src"
    if os.path.isdir(centos_headers_dir):
        linux_headers = [
            os.path.join(centos_headers_dir, d) for d in os.listdir(centos_headers_dir)
        ]
    else:
        linux_headers = [
            os.path.join(debian_headers_dir, d) for d in os.listdir(debian_headers_dir)
            if d.startswith("linux-")
        ]

    bpf_dir = os.path.join(".", "pkg", "ebpf")
    c_dir = os.path.join(bpf_dir, "c")

    flags = [
        '-D__KERNEL__',
        '-DCONFIG_64BIT',
        '-D__BPF_TRACING__',
        '-Wno-unused-value',
        '-Wno-pointer-sign',
        '-Wno-compare-distinct-pointer-types',
        '-Wunused',
        '-Wall',
        '-Werror',
        '-O2',
        '-emit-llvm',
        '-c',
        os.path.join(c_dir, "tracer-ebpf.c"),
    ]

    # Mapping used by the kernel, from https://elixir.bootlin.com/linux/latest/source/scripts/subarch.include
    arch = check_output('''uname -m | sed -e s/i.86/x86/ -e s/x86_64/x86/ \
                    -e s/sun4u/sparc64/ \
                    -e s/arm.*/arm/ -e s/sa110/arm/ \
                    -e s/s390x/s390/ -e s/parisc64/parisc/ \
                    -e s/ppc.*/powerpc/ -e s/mips.*/mips/ \
                    -e s/sh[234].*/sh/ -e s/aarch64.*/arm64/ \
                    -e s/riscv.*/riscv/''', shell=True).decode('utf-8').strip()

    subdirs = [
        "include",
        "include/uapi",
        "include/generated/uapi",
        "arch/{}/include".format(arch),
        "arch/{}/include/uapi".format(arch),
        "arch/{}/include/generated".format(arch),
    ]

    for d in linux_headers:
        for s in subdirs:
            flags.extend(["-isystem", os.path.join(d, s)])

    cmd = "clang {flags} -o - | llc -march=bpf -filetype=obj -o '{file}'"

    commands = []

    # Build both the standard and debug version
    obj_file = os.path.join(c_dir, "tracer-ebpf.o")
    commands.append(cmd.format(
        flags=" ".join(flags),
        file=obj_file
    ))

    debug_obj_file = os.path.join(c_dir, "tracer-ebpf-debug.o")
    commands.append(cmd.format(
        flags=" ".join(flags + ["-DDEBUG=1"]),
        file=debug_obj_file
    ))

    if install:
        # Now update the assets stored in the go code
        commands.append("go get -u github.com/jteeuwen/go-bindata/...")

        assets_cmd = os.environ["GOPATH"]+"/bin/go-bindata -pkg bytecode -prefix '{c_dir}' -modtime 1 -o '{go_file}' '{obj_file}' '{debug_obj_file}' '{tcp_queue_length_kern_c_file}' '{tcp_queue_length_kern_user_h_file}'"
        go_file = os.path.join(bpf_dir, "bytecode", "tracer-ebpf.go")
        commands.append(assets_cmd.format(
            c_dir=c_dir,
            go_file=go_file,
            obj_file=obj_file,
            debug_obj_file=debug_obj_file,
            tcp_queue_length_kern_c_file=os.path.join(c_dir, "tcp-queue-length-kern.c"),
            tcp_queue_length_kern_user_h_file=os.path.join(c_dir, "tcp-queue-length-kern-user.h"),
        ))

        commands.append("gofmt -w -s {go_file}".format(go_file=go_file))

    for cmd in commands:
        ctx.run(cmd)


def build_ebpf_builder(ctx):
    """build_ebpf_builder builds the docker image for the ebpf builder
    """

    cmd = "docker build -t {image} -f {file} ."

    if should_use_sudo(ctx):
        cmd = "sudo " + cmd

    ctx.run(cmd.format(image=EBPF_BUILDER_IMAGE, file=EBPF_BUILDER_FILE))

def is_root():
    return os.getuid() == 0

def should_use_sudo(ctx):
    # We are already root
    if is_root():
        return False

    with open(os.devnull, 'w') as FNULL:
        try:
            check_output(['docker', 'info'], stderr=FNULL)
        except CalledProcessError:
            return True

    return False


@contextlib.contextmanager
def tempdir():
    """
    Helper to create a temp directory and clean it
    """
    dirpath = tempfile.mkdtemp()
    try:
        yield dirpath
    finally:
        shutil.rmtree(dirpath)
