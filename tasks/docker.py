"""
Docker related tasks
"""
from __future__ import print_function
from shutil import copy2
import os

from invoke import task
from invoke.exceptions import Exit

from .utils import bin_name
from .dogstatsd import STATIC_BIN_PATH

# constants
DOGSTATSD_TAG = "datadog/dogstatsd:master"


@task
def integration_tests(ctx, skip_build=False):
    """
    Run docker integration tests
    """
    target = os.path.join(STATIC_BIN_PATH, bin_name("dogstatsd"))
    if not skip_build:
        # postpone the import otherwise `build` will be added to the docker
        # namespace
        from .dogstatsd import build
        print("Building dogstatsd")
        build(ctx, static=True)

    print("Building dogstatsd container")
    if not os.path.exists(target):
        raise Exit(1)

    copy2(target, "Dockerfiles/dogstatsd/alpine/dogstatsd")
    ctx.run("docker build -t {} Dockerfiles/dogstatsd/alpine/".format(DOGSTATSD_TAG))

    print("Starting docker integration tests")
    env = {"DOCKER_IMAGE": DOGSTATSD_TAG}
    ctx.run("./test/integration/docker/dsd_alpine_listening.sh", env=env)
