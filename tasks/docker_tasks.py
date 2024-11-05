"""
Docker related tasks
"""

import os
import shutil
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit

from tasks.dogstatsd import DOGSTATSD_TAG


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
    ctx.run(f"{python_command} ./test/integration/docker/dsd_listening.py", env=env)


@task
def dockerize_test(ctx, binary, skip_cleanup=False):
    """
    Run a go test in a remote docker environment and pipe its output to stdout.
    Host and target systems must be identical (test is built on the host).
    """
    import docker

    client = docker.from_env()
    temp_folder = tempfile.mkdtemp(prefix="ddtest-")

    ctx.run(f"cp {binary} {temp_folder}/test.bin")

    with open(f"{temp_folder}/Dockerfile", 'w') as stream:
        stream.write(
            """FROM public.ecr.aws/docker/library/ubuntu:20.04
# Install Docker
COPY --from=public.ecr.aws/docker/library/docker:26.1-cli /usr/local/bin/docker /usr/bin/docker

# Install Docker Compose
ARG COMPOSE_VERSION=2.26.1
ARG COMPOSE_SHA256=2f61856d1b8c9de29ffdaedaa1c6d0a5fc5c79da45068f1f4310feed8d3a3f61
RUN apt-get update && apt-get install -y ca-certificates curl
RUN curl -SL "https://github.com/docker/compose/releases/download/v${COMPOSE_VERSION}/docker-compose-linux-x86_64" -o /usr/bin/compose
RUN echo "${COMPOSE_SHA256} /usr/bin/compose" | sha256sum --check
RUN chmod +x /usr/bin/compose

# Final settings
ENV DOCKER_DD_AGENT=yes
WORKDIR /
CMD /test.bin
COPY test.bin /test.bin
"""
        )
        # Handle optional testdata folder
        if os.path.isdir("./testdata"):
            ctx.run(f"cp -R testdata {temp_folder}")
            stream.write("COPY testdata /testdata")

    test_image, _ = client.images.build(path=temp_folder, rm=True)

    scratch_volume = client.volumes.create()

    test_container = client.containers.run(
        test_image.id,
        detach=True,
        pid_mode="host",  # For origin detection
        cgroupns="host",  # To allow proper network mode detection in integration tests
        environment=["SCRATCH_VOLUME_NAME=" + scratch_volume.name, "SCRATCH_VOLUME_PATH=/tmp/scratch"],
        volumes={
            '/var/run/docker.sock': {'bind': '/var/run/docker.sock', 'mode': 'ro'},
            '/proc': {'bind': '/host/proc', 'mode': 'ro'},
            '/sys/fs/cgroup': {'bind': '/host/sys/fs/cgroup', 'mode': 'ro'},
            scratch_volume.name: {'bind': '/tmp/scratch', 'mode': 'rw'},
        },
    )

    exit_code = test_container.wait()['StatusCode']

    stdout_logs = test_container.logs(stdout=True, stderr=False, stream=False).decode(sys.stdout.encoding)
    stderr_logs = test_container.logs(stdout=False, stderr=True, stream=False).decode(sys.stderr.encoding)

    print(stdout_logs)
    print(stderr_logs, file=sys.stderr)

    skip_cleanup = True
    if not skip_cleanup:
        shutil.rmtree(temp_folder)
        test_container.remove(v=True, force=True)
        scratch_volume.remove(force=True)
        client.images.remove(test_image.id)

    if exit_code != 0:
        raise Exit(code=exit_code)


@task
def delete(ctx, org, image, tag, token):
    print(f"Deleting {org}/{image}:{tag}")
    ctx.run(
        f"curl 'https://hub.docker.com/v2/repositories/{org}/{image}/tags/{tag}/' -X DELETE -H 'Authorization: JWT {token}' &>/dev/null"
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

    with open(dockerfile) as f:
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
        print(f"Ignoring intermediate stage names: {', '.join(stages)}")
        images -= stages

    print(f"Pulling following base images: {', '.join(images)} (content-trust:{signed_pull})")

    pull_env = {}
    if signed_pull:
        pull_env["DOCKER_CONTENT_TRUST"] = "1"

    for i in images:
        ctx.run(f"docker pull {i}", env=pull_env)
