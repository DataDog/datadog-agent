"""
Cluster Agent for Cloud Foundry tasks
"""

import os

from invoke import task

from tasks.cluster_agent_helpers import build_common, clean_common, refresh_assets_common, version_common

# constants
BIN_PATH = os.path.join(".", "bin", "datadog-cluster-agent-cloudfoundry")


@task
def build(ctx, rebuild=False, build_include=None, build_exclude=None, race=False, development=True, skip_assets=False):
    """
    Build Cluster Agent for Cloud Foundry

     Example invokation:
        inv cluster-agent-cloudfoundry.build
    """
    build_common(
        ctx,
        BIN_PATH,
        "cluster-agent-cloudfoundry",
        "-cloudfoundry",
        rebuild,
        build_include,
        build_exclude,
        race,
        development,
        skip_assets,
    )


@task
def refresh_assets(ctx, development=True):
    """
    Clean up and refresh cluster agent's assets and config files
    """
    refresh_assets_common(ctx, BIN_PATH, [], development)


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):  # noqa: U100
    """
    Run integration tests for cluster-agent-cloudfoundry
    """
    pass  # TODO


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    clean_common(ctx, "datadog-cluster-agent")


@task
def version(ctx, url_safe=False, git_sha_length=7):
    """
    Get the agent version.
    url_safe: get the version that is able to be addressed as a url
    git_sha_length: different versions of git have a different short sha length,
                    use this to explicitly set the version
                    (the windows builder and the default ubuntu version have such an incompatibility)
    """
    version_common(ctx, url_safe, git_sha_length)
