"""
Docker related tasks
"""
from __future__ import print_function, absolute_import
import tempfile
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit
import docker

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
    ctx.run("./test/integration/docker/dsd_alpine_listening.sh", env=env)


@task
def dockerize_test(ctx, binary, skip_cleanup=False):
    """
    Run a go test in a remote docker environment and pipe its output to stdout
    """
    client = docker.from_env()
    temp_folder = tempfile.mkdtemp(prefix="ddtest-")

    ctx.run("cp %s %s/test.bin" % (binary, temp_folder))
    # TODO: handle testdata folder if present

    with open("%s/Dockerfile" % temp_folder, 'w') as stream:
        stream.write("""FROM debian:stretch-slim
ENV DOCKER_DD_AGENT=yes
CMD /test.bin
COPY test.bin /test.bin
""")

    test_image = client.images.build(path=temp_folder, rm=True)

    test_container = client.containers.run(
        test_image.id,
        detach=True,
        volumes={'/var/run/docker.sock': {'bind': '/var/run/docker.sock', 'mode': 'ro'}})

    exit_code = test_container.wait()

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
        client.images.remove(test_image.id)

    if exit_code != 0:
        raise Exit(exit_code)
