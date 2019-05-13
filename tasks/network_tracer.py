import datetime
import glob
import os
import contextlib
import shutil
import tempfile

from invoke import task
from subprocess import check_output, CalledProcessError

from .utils import bin_name, get_build_flags, REPO_PATH, get_version, get_git_branch_name, get_go_version, get_git_commit
from .build_tags import get_default_build_tags

BIN_DIR = os.path.join(".", "bin", "network-tracer")
BIN_PATH = os.path.join(BIN_DIR, bin_name("network-tracer", android=False))

EBPF_BUILDER_IMAGE = 'datadog/tracer-bpf-builder'
EBPF_BUILDER_FILE = os.path.join(".", "Dockerfiles", "network-tracer", "Dockerfile-ebpf")

BPF_TAG = "linux_bpf"

@task
def build(ctx, race=False, rebuild_ebpf_builder=False, incremental_build=False, puppy=False):
    """
    Build the network_tracer
    """

    if rebuild_ebpf_builder:
        build_ebpf_builder(ctx)

    build_object_files(ctx)

    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx),
        "GoVersion": get_go_version(),
        "GitBranch": get_git_branch_name(),
        "GitCommit": get_git_commit(),
        "BuildDate": datetime.datetime.now().strftime("%FT%T%z"),
    }

    ldflags, gcflags, env = get_build_flags(ctx)

    # Add custom ld flags
    ldflags += ' '.join(["-X '{name}={value}'".format(name=main+key, value=value) for key, value in ld_vars.items()])
    build_tags = get_default_build_tags(puppy=puppy) + [BPF_TAG]

    # TODO static option
    cmd = 'go build {race_opt} {build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/network-tracer'

    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-i" if incremental_build else "-a",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": BIN_PATH,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)


@task
def test(ctx):
    """
    Run tests on eBPF parts
    """

    pkg = os.path.join(REPO_PATH, "pkg", "ebpf", "...")

    # Pass along the PATH env variable to retrieve the go binary path
    path = os.environ['PATH']

    cmd = 'sudo -E PATH={path} go test -v -tags "{bpf_tag}" {pkg}'
    ctx.run(cmd.format(path=path, bpf_tag=BPF_TAG, pkg=pkg))


@task
def build_builder_image(ctx):
    """
    Builds the ebpf builder image
    """
    build_ebpf_builder(ctx)


@task
def nettop(ctx):
    """
    Build and run the `nettop` utility for testing
    """
    build_object_files(ctx)

    cmd = 'go build -a -tags "linux_bpf" -o {bin_path} {path}'
    bin_path = os.path.join(BIN_DIR, "nettop")
    # Build
    ctx.run(cmd.format(path=os.path.join(REPO_PATH, "pkg", "ebpf", "nettop"), bin_path=bin_path))
    # Run
    ctx.run("sudo {}".format(bin_path))


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
def build_docker_image(ctx, image_name):
    """
    Build a network-tracer-agent Docker image (development only)
    """

    dev_file = os.path.join(".", "Dockerfiles", "network-tracer", "Dockerfile-tracer-dev")
    cmd = "docker build {directory} -t {image_name} -f {file}"

    # Build in a temporary directory to make the docker build context small
    with tempdir() as d:
        shutil.copy(BIN_PATH, d)
        ctx.run(cmd.format(directory=d, image_name=image_name, file=dev_file))


@task
def codegen(ctx):
    """codegen handles retrieving the easyjson dependency and rebuilding
    the easyjson files
    """

    ctx.run("go get -u github.com/mailru/easyjson/...")
    path = os.path.join(".", "pkg", "ebpf", "event_common.go")
    ctx.run("easyjson {}".format(path))


def build_ebpf_builder(ctx):
    """build_ebpf_builder builds the docker image for the ebpf builder
    """

    cmd = "docker build -t {image} -f {file} ."

    if should_use_sudo(ctx):
        cmd = "sudo " + cmd

    ctx.run(cmd.format(image=EBPF_BUILDER_IMAGE, file=EBPF_BUILDER_FILE))


def build_object_files(ctx):
    """build_object_files_only builds only the eBPF object
    (without rebuilding the docker image builder)
    """

    makeCmd = "make -f /ebpf/c/tracer-ebpf.mk build install"
    args = {
        "circle_url": "TODO",
        "builder": EBPF_BUILDER_IMAGE,
        "makeCmd": makeCmd
    }
    cmd = "docker run --rm -e CIRCLE_BUILD_URL={circle_url} \
            -v $(pwd)/pkg/ebpf:/ebpf/ \
            --workdir=/ebpf \
            {builder} \
            {makeCmd}"

    if should_use_sudo(ctx):
        cmd = "sudo " + cmd

    ctx.run(cmd.format(**args))


def should_use_sudo(ctx):
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
