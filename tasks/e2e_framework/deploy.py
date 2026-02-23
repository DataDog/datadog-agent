from typing import Any

import boto3
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from . import tool


def deploy(
    ctx: Context,
    scenario_name: str,
    config_path: str | None = None,
    app_key_required: bool = False,
    stack_name: str | None = None,
    pipeline_id: str | None = None,
    install_agent: bool | None = None,
    install_installer: bool | None = None,
    install_workload: bool | None = None,
    agent_version: str | None = None,
    debug: bool | None = False,
    extra_flags: dict[str, Any] | None = None,
    use_fakeintake: bool | None = False,
    full_image_path: str | None = None,
    cluster_agent_full_image_path: str | None = None,
    agent_flavor: str | None = None,
    agent_config_path: str | None = None,
    agent_env: str | None = None,
    helm_config: str | None = None,
    local_package: str | None = None,
) -> str:
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    flags = extra_flags if extra_flags else {}

    if install_agent is None:
        install_agent = tool.get_default_agent_install()
    flags["ddagent:deploy"] = install_agent and not install_installer
    flags["ddupdater:deploy"] = install_installer

    if install_workload is None:
        install_workload = tool.get_default_workload_install()
    flags["ddtestworkload:deploy"] = install_workload

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}") from e

    flags["scenario"] = scenario_name
    flags["ddagent:version"] = agent_version
    flags["ddagent:flavor"] = agent_flavor
    flags["ddagent:fakeintake"] = use_fakeintake
    flags["ddagent:fullImagePath"] = full_image_path
    flags["ddagent:clusterAgentFullImagePath"] = cluster_agent_full_image_path
    flags["ddagent:configPath"] = agent_config_path
    flags["ddagent:extraEnvVars"] = agent_env
    flags["ddagent:helmConfig"] = helm_config
    flags["ddagent:localPackage"] = local_package

    flags["ddagent:pipeline_id"] = "" if pipeline_id is None else pipeline_id

    if install_agent:
        flags["ddagent:apiKey"] = config.get_api_key(cfg)

    # When using fakeintake, enable dual shipping to send data to both fakeintake and Datadog
    # Otherwise pulumi will configure the agent to send data directly to fakeintake
    # this is breakind UI/console related features that require communication with Datadog backend
    # see go package agent.HelmValues.configureFakeintake for more details
    if use_fakeintake is True:
        flags["ddagent:dualshipping"] = True

    # add stack params values
    stackParams = cfg.get_stack_params()
    for namespace in stackParams:
        for key, value in stackParams[namespace].items():
            flags[f"{namespace}:{key}"] = value

    if app_key_required:
        flags["ddagent:appKey"] = config.get_app_key(cfg)

    return _deploy(
        ctx,
        stack_name,
        flags,
        debug,
        cfg.get_pulumi().logLevel,
        cfg.get_pulumi().logToStdErr,
    )


@task
def check_s3_image_exists(_, pipeline_id: str, deploy_job: str):
    """
    Verify if an image exists in the s3 repository to create a vm
    """
    # Job to s3 directory mapping
    deploy_job_to_s3 = {
        # Deb
        "deploy_deb_testing-a7_x64": f"apttesting.datad0g.com/datadog-agent/pipeline-{pipeline_id}-a7/dists/stable-x86_64/7/binary-amd64",
        "deploy_deb_testing-a7_arm64": f"apttesting.datad0g.com/datadog-agent/pipeline-{pipeline_id}-a7/dists/stable-arm64/7/binary-arm64",
        "deploy_deb_testing-a6_x64": f"apttesting.datad0g.com/datadog-agent/pipeline-{pipeline_id}-a6/dists/stable-x86_64/6/binary-amd64",
        "deploy_deb_testing-a6_arm64": f"apttesting.datad0g.com/datadog-agent/pipeline-{pipeline_id}-a6/dists/stable-arm64/6/binary-arm64",
        # Rpm
        "deploy_rpm_testing-a7_x64": f"yumtesting.datad0g.com/testing/pipeline-{pipeline_id}-a7/7/x86_64",
        "deploy_rpm_testing-a7_arm64": f"yumtesting.datad0g.com/testing/pipeline-{pipeline_id}-a7/7/aarch64",
        "deploy_rpm_testing-a6_x64": f"yumtesting.datad0g.com/testing/pipeline-{pipeline_id}-a6/6/x86_64",
        "deploy_rpm_testing-a6_arm64": f"yumtesting.datad0g.com/testing/pipeline-{pipeline_id}-a6/6/aarch64",
        # Suse
        "deploy_suse_rpm_testing_x64-a7": f"yumtesting.datad0g.com/suse/testing/pipeline-{pipeline_id}-a7/7/x86_64",
        "deploy_suse_rpm_testing_arm64-a7": f"yumtesting.datad0g.com/testing/pipeline-{pipeline_id}-a7/7/aarch64",
        "deploy_suse_rpm_testing_x64-a6": f"yumtesting.datad0g.com/testing/pipeline-{pipeline_id}-a6/6/x86_64",
        "deploy_suse_rpm_testing_arm64-a6": f"yumtesting.datad0g.com/testing/pipeline-{pipeline_id}-a6/6/aarch64",
        # Windows
        "deploy_windows_testing-a7": f"dd-agent-mstesting/pipelines/A7/{pipeline_id}",
        "deploy_windows_testing-a6": f"dd-agent-mstesting/pipelines/A6/{pipeline_id}",
        # macOS
        "deploy_dmg_testing-a7_x64": f"dd-agent-macostesting/ci/datadog-agent/pipeline-{pipeline_id}",
    }

    bucket_path = deploy_job_to_s3[deploy_job]
    delim = bucket_path.find("/")
    bucket = bucket_path[:delim]
    path = bucket_path[delim + 1 :]

    s3 = boto3.client("s3")
    response = s3.list_objects_v2(Bucket=bucket, Prefix=path)
    exists = "Contents" in response

    assert exists, f"Latest job {deploy_job} is outdated, use `inv retry-job {pipeline_id} {deploy_job}` to run it again or use --no-verify to force deploy"


# creates a stack with the given stack_name if it doesn't already exists
def _create_stack(ctx: Context, stack_name: str, global_flags: str):
    result = ctx.run(f"pulumi {global_flags} stack ls --all", hide="stdout")
    if not result:
        return

    stacks = result.stdout.splitlines()[1:]  # skip header
    for stack in stacks:
        # the stack has an asterisk if it is currently selected
        ls_stack_name = stack.split(" ")[0].rstrip("*")
        if ls_stack_name == stack_name:
            return

    ctx.run(f"pulumi {global_flags} stack init --no-select {stack_name}")


def _deploy(
    ctx: Context,
    stack_name: str | None,
    flags: dict[str, Any],
    debug: bool | None,
    log_level: int | None,
    log_to_stderr: bool | None,
) -> str:
    stack_name = tool.get_stack_name(stack_name, flags["scenario"])
    # make sure the stack name is safe
    stack_name = stack_name.replace(" ", "-").lower()
    global_flags_array: list[str] = []
    up_flags = ""

    # Check we are in a pulumi project
    global_flags_array.append(tool.get_pulumi_dir_flag())

    # Building run func parameters
    for key, value in flags.items():
        if value is not None:
            up_flags += f' -c "{key}={value}"'

    should_log = debug or log_level is not None or log_to_stderr
    if log_level is None:
        log_level = 3
    if log_to_stderr is None:
        # default to true if debug is enabled
        log_to_stderr = debug
    if should_log:
        if log_to_stderr:
            global_flags_array.append("--logtostderr")
        global_flags_array.append(f"-v {log_level}")
        if debug:
            up_flags += " --debug"

    global_flags = " ".join(global_flags_array)
    _create_stack(ctx, stack_name, global_flags)
    cmd = f"pulumi {global_flags} up --yes -s {stack_name} {up_flags}"

    pty = True
    if tool.is_windows():
        pty = False
    ctx.run(cmd, pty=pty)
    return stack_name
