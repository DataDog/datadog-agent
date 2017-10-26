"""
Docker related tasks
"""
from __future__ import print_function

from invoke import task

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
