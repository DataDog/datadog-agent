"""
Docker related tasks
"""
from __future__ import print_function
from shutil import copy2

from invoke import task

from .dogstatsd import build

# constants
DOGSTATSD_TAG = "datadog/dogstatsd:master"


@task
def build_dogstatsd(ctx, skip_build=False):
    """
    Build the static version of Dogstasd to ship with Docker images
    """
    if not skip_build:
        build(ctx)

    copy2("bin/static/dogstatsd", "Dockerfiles/dogstatsd/alpine/")
    ctx.run("docker build -t {} Dockerfiles/dogstatsd/alpine/".format(DOGSTATSD_TAG))


@task(pre=[build_dogstatsd])
def integration_tests(ctx):
    """
    Run docker integration tests
    """
    print("Starting docker integration tests")
    env = {"DOCKER_IMAGE": DOGSTATSD_TAG}
    ctx.run("./test/integration/docker/dsd_alpine_listening.sh", env=env)
