"""
Docker related tasks
"""
from __future__ import print_function, absolute_import
import tempfile
import shutil
import sys
import os
import re
import signal

from invoke import task
from invoke.exceptions import Exit

from .dogstatsd import DOGSTATSD_TAG


@task
def test(ctx):
    """
    Run docker tests
    """
    ctx.run("python ./Dockerfiles/agent/secrets-helper/test_readsecret.py")


@task
def integration_tests(ctx, skip_image_build=False, skip_build=False):
    """
    Run docker integration tests
    """
    if not skip_image_build:
        # postpone the import otherwise `image_build` will be added to the docker
        # namespace
        from .dogstatsd import image_build
        image_build(ctx, skip_build=skip_build)

    print("Starting docker integration tests")
    env = {"DOCKER_IMAGE": DOGSTATSD_TAG}
    ctx.run("python ./test/integration/docker/dsd_listening.py", env=env)


@task
def dockerize_test(ctx, binary, skip_cleanup=False):
    """
    Run a go test in a remote docker environment and pipe its output to stdout.
    Host and target systems must be identical (test is built on the host).
    """
    import docker

    client = docker.from_env()
    temp_folder = tempfile.mkdtemp(prefix="ddtest-")

    ctx.run("cp %s %s/test.bin" % (binary, temp_folder))

    with open("%s/Dockerfile" % temp_folder, 'w') as stream:
        stream.write("""FROM debian:stretch-slim
ENV DOCKER_DD_AGENT=yes
WORKDIR /
ADD https://github.com/docker/compose/releases/download/1.16.1/docker-compose-Linux-x86_64 /bin/docker-compose
RUN echo "1804b0ce6596efe707b9cab05d74b161833ed503f0535a937dd5d17bea8fc50a  /bin/docker-compose" > sum && \
    sha256sum -c sum && \
    chmod +x /bin/docker-compose
CMD /test.bin
COPY test.bin /test.bin
""")
        # Handle optional testdata folder
        if os.path.isdir("./testdata"):
            ctx.run("cp -R testdata %s" % temp_folder)
            stream.write("COPY testdata /testdata")

    test_image, _ = client.images.build(path=temp_folder, rm=True)

    scratch_volume = client.volumes.create()

    test_container = client.containers.run(
        test_image.id,
        detach=True,
        pid_mode="host",  # For origin detection
        environment=[
            "SCRATCH_VOLUME_NAME=" + scratch_volume.name,
            "SCRATCH_VOLUME_PATH=/tmp/scratch",
        ],
        volumes={'/var/run/docker.sock': {'bind': '/var/run/docker.sock', 'mode': 'ro'},
                 '/proc': {'bind': '/host/proc', 'mode': 'ro'},
                 '/sys/fs/cgroup': {'bind': '/host/sys/fs/cgroup', 'mode': 'ro'},
                 scratch_volume.name: {'bind': '/tmp/scratch', 'mode': 'rw'}})

    exit_code = test_container.wait()['StatusCode']

    print(test_container.logs(
        stdout=True,
        stderr=False,
        stream=False))

    sys.stderr.write(test_container.logs(
        stdout=False,
        stderr=True,
        stream=False))

    if not skip_cleanup:
        shutil.rmtree(temp_folder)
        test_container.remove(v=True, force=True)
        scratch_volume.remove(force=True)
        client.images.remove(test_image.id)

    if exit_code != 0:
        raise Exit(code=exit_code)


@task
def dockerize_omnibus(ctx, skip_cleanup=False, clear_cache=False, cache_folder="~/.omni-build"):
    """
    Run omnibus in a docker environment and pipe its output to stdout.
    """
    import docker
    temp_folder = None
    build_image = None
    build_container = None

    cache_folder = os.path.expanduser(cache_folder)

    if clear_cache and os.path.exists(cache_folder):
        shutil.rmtree(temp_folder)

    def cleanup():
        if skip_cleanup:
            return
        if build_container:
            build_container.remove(v=True, force=True)
        if build_image:
            client.images.remove(build_image.id)
        if temp_folder:
            shutil.rmtree(temp_folder)

    def signal_handler(sig, frame):
        cleanup()
    signal.signal(signal.SIGINT, signal_handler)

    client = docker.from_env()
    temp_folder = tempfile.mkdtemp(prefix="ddtest-")

    with open("%s/run.sh" % temp_folder, 'w') as stream:
        stream.write("""#!/bin/bash
ls $STATE_DIR/venv/bin/activate || virtualenv $STATE_DIR/venv
. $STATE_DIR/venv/bin/activate
pip install -r requirements.txt

rm -rf $STATE_DIR/omnibus/src/datadog-agent/src/github.com/DataDog/datadog-agent

invoke -e agent.omnibus-build \
  --skip-sign --skip-deps \
  --base-dir=$STATE_DIR/omnibus \
  --gem-path=$STATE_DIR/gems
""")

    with open("%s/Dockerfile" % temp_folder, 'w') as stream:
        stream.write("""FROM datadog/datadog-agent-runner-circle:latest
ENV STATE_DIR="/tmp/omni-build"

ADD run.sh /
RUN chmod +x /run.sh \
 && git config --global user.email "omnibus@datadoghq.com" \
 && git config --global user.name "Omnibus builder"

WORKDIR /go/src/github.com/DataDog/datadog-agent/
CMD /run.sh
""")

    build_image, _ = client.images.build(path=temp_folder, rm=True)
    build_container = client.containers.run(
        build_image.id,
        detach=True,
        environment=[],
        volumes={
            os.getcwd(): {'bind': '/go/src/github.com/DataDog/datadog-agent', 'mode': 'rw'},
            cache_folder: {'bind': '/tmp/omni-build', 'mode': 'rw'},
        })

    for line in build_container.logs(stdout=True, stderr=True, stream=True, follow=True):
        sys.stdout.write(line)
    exit_code = build_container.wait()['StatusCode']

    cleanup()

    if exit_code != 0:
        raise Exit(code=exit_code)


@task
def mirror_image(ctx, src_image, dst_image="datadog/docker-library", dst_tag="auto"):
    """
    Pull an upstream image and mirror it to our docker-library repository
    for integration tests. Tag format should be A-Z_n_n_n
    """
    if dst_tag == "auto":
        # Autogenerate tag
        match = re.search('([^:\/\s]+):[v]?(.*)$', src_image)
        if not match:
            print("Cannot guess destination tag for {}, please provide a --dst-tag option".format(src_image))
            raise Exit(code=1)
        dst_tag = "_".join(match.groups()).replace(".", "_")

    dst = "{}:{}".format(dst_image, dst_tag)
    publish(src_image, dst)


@task
def publish(ctx, src, dst, signed_pull=False, signed_push=False):
    print("Uploading {} to {}".format(src, dst))

    pull_env = {}
    if signed_pull:
        pull_env["DOCKER_CONTENT_TRUST"] = "1"
    ctx.run(
        "docker pull {src} && docker tag {src} {dst}".format(src=src, dst=dst),
        env=pull_env
    )

    push_env = {}
    if signed_push:
        push_env["DOCKER_CONTENT_TRUST"] = "1"
    ctx.run(
        "docker push {dst}".format(dst=dst),
        env=push_env
    )

@task
def pull_base_images(ctx, dockerfile, signed_pull=True):
    """
    Pulls the base images for a given Dockerfile, with
    content trust enabled by default, to ensure the base
    images are signed
    """
    images = set()
    stages = set()

    with open(dockerfile, "r") as f:
        for line in f:
            words = line.split()
            # Get source images
            if len(words) < 2 or words[0].lower() != "from":
                continue
            images.add(words[1])
            # Get stage names to remove them from pull
            if len(words) < 4 or words[2].lower() != "as":
                continue
            stages.add(words[3])

    if stages:
        print("Ignoring intermediate stage names: {}".format(", ".join(stages)))
        images -= stages

    print("Pulling following base images: {}".format(", ".join(images)))

    pull_env = {}
    if signed_pull:
        pull_env["DOCKER_CONTENT_TRUST"] = "1"

    for i in images:
        ctx.run("docker pull {}".format(i), env=pull_env)
