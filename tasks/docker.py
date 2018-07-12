"""
Docker related tasks
"""
from __future__ import print_function, absolute_import
import tempfile
import shutil
import sys
import os
import re

from invoke import task
from invoke.exceptions import Exit

from .dogstatsd import DOGSTATSD_TAG


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
def publish(ctx, src, dst):
    print("Uploading {} to {}".format(src, dst))
    ctx.run("docker pull {src} && docker tag {src} {dst} && docker push {dst}".format(
        src=src,
        dst=dst)
    )
