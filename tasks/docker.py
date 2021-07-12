"""
Docker related tasks
"""


import os
import shutil
import sys
import tempfile
import time

from invoke import task
from invoke.exceptions import Exit

from .dogstatsd import DOGSTATSD_TAG


def retry_run(ctx, *args, **kwargs):
    remaining_retries = 5
    while True:
        warn = True
        if remaining_retries == 0:
            warn = False

        r = ctx.run(*args, warn=warn, **kwargs)

        if r.ok:
            return r

        # Pause between retries. Hope it helps.
        time.sleep(5)

        remaining_retries -= 1

    return r


@task
def test(ctx):
    """
    Run docker tests
    """
    ctx.run("python3 ./Dockerfiles/agent/secrets-helper/test_readsecret.py")


@task
def integration_tests(ctx, skip_image_build=False, skip_build=False, python_command="python3"):
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
    ctx.run("{} ./test/integration/docker/dsd_listening.py".format(python_command), env=env)


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
        stream.write(
            """FROM docker/compose:debian-1.28.3
ENV DOCKER_DD_AGENT=yes
WORKDIR /
CMD /test.bin
COPY test.bin /test.bin
"""
        )
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
        environment=["SCRATCH_VOLUME_NAME=" + scratch_volume.name, "SCRATCH_VOLUME_PATH=/tmp/scratch",],
        volumes={
            '/var/run/docker.sock': {'bind': '/var/run/docker.sock', 'mode': 'ro'},
            '/proc': {'bind': '/host/proc', 'mode': 'ro'},
            '/sys/fs/cgroup': {'bind': '/host/sys/fs/cgroup', 'mode': 'ro'},
            scratch_volume.name: {'bind': '/tmp/scratch', 'mode': 'rw'},
        },
    )

    exit_code = test_container.wait()['StatusCode']

    print(test_container.logs(stdout=True, stderr=False, stream=False))

    sys.stderr.write(test_container.logs(stdout=False, stderr=True, stream=False).decode(sys.stderr.encoding))

    if not skip_cleanup:
        shutil.rmtree(temp_folder)
        test_container.remove(v=True, force=True)
        scratch_volume.remove(force=True)
        client.images.remove(test_image.id)

    if exit_code != 0:
        raise Exit(code=exit_code)


@task
def delete(ctx, org, image, tag, token):
    print("Deleting {org}/{image}:{tag}".format(org=org, image=image, tag=tag))
    ctx.run(
        "curl 'https://hub.docker.com/v2/repositories/{org}/{image}/tags/{tag}/' -X DELETE -H 'Authorization: JWT {token}' &>/dev/null".format(
            org=org, image=image, tag=tag, token=token
        )
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
