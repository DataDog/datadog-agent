import os
import subprocess
from typing import Any

from invoke.context import Context
from invoke.exceptions import Exit

from tasks.e2e_framework import tool
from tasks.e2e_framework.deploy import deploy as common_deploy

# The ECR registries the Agent images can be pulled from
AGENT_QA_REGISTRY = "669783387624.dkr.ecr.us-east-1.amazonaws.com"
SANDBOX_REGISTRY = "376334461865.dkr.ecr.us-east-1.amazonaws.com"


def deploy(
    ctx: Context,
    scenario_name: str,
    config_path: str | None = None,
    key_pair_required: bool = False,
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
    deploy_job: str | None = None,
    full_image_path: str | None = None,
    cluster_agent_full_image_path: str | None = None,
    agent_flavor: str | None = None,
    agent_config_path: str | None = None,
    agent_env: str | None = None,
    helm_config: str | None = None,
    local_package: str | None = None,
    pulumi_extra_args: str = "",
    pulumi_env: dict[str, str] | None = None,
    needs_agent_containers: bool = True,
    extra_image_paths: list[str | None] | None = None,
) -> str:
    """
    extra_image_paths are the images of the scenarios deploying several Agents, like benchmarkeks, whose registries
    need pull credentials just like those of full_image_path and cluster_agent_full_image_path.
    """
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    flags = extra_flags if extra_flags else {}

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}:{e}") from e

    # Keep ~/.aws/config in sync: add the SSO profile if it's missing (e.g. after a role
    # rename like account-admin -> account-admin-8h). No-op if already present.
    from tasks.e2e_framework.setup.aws import DEFAULT_AWS_REGION, ECR_CACHE_PROFILE, setup_aws_sso_config

    setup_aws_sso_config(cfg, interactive=False)

    awsKeyPairName = cfg.get_aws().keyPairName

    flags["ddinfra:aws/defaultKeyPairName"] = awsKeyPairName
    aws_account = cfg.get_aws().get_account()
    flags.setdefault("ddinfra:env", "aws/" + aws_account)

    # Verify image deployed and not outdated in s3
    if deploy_job is not None and pipeline_id is not None:
        cmd = f"inv -e e2e.check-s3-image-exists --pipeline-id={pipeline_id} --deploy-job={deploy_job}"
        cmd = tool.get_aws_wrapper(aws_account) + cmd
        output = ctx.run(cmd, warn=True)

        # The command already has a traceback
        if not output or output.return_code != 0:
            raise Exit(code=1)

    if cfg.get_aws().teamTag is None or cfg.get_aws().teamTag == "":
        raise Exit(
            "Error in config, missing configParams.aws.teamTag. Run `inv setup` again and provide a valid team name"
        )

    if key_pair_required and cfg.get_options().checkKeyPair:
        _check_key_pair(awsKeyPairName)

    image_paths = [full_image_path, cluster_agent_full_image_path, *(extra_image_paths or [])]

    def uses_registry(registry: str) -> bool:
        return any(path and path.startswith(registry + "/") for path in image_paths)

    # The registries to pull from, with the command printing their password: the images may
    # come from both, e.g. two Agent versions compared by benchmarkeks.
    pull_registries = {}
    if needs_agent_containers and (pipeline_id is not None or uses_registry(AGENT_QA_REGISTRY)):
        pull_registries[AGENT_QA_REGISTRY] = (
            f"aws-vault exec {ECR_CACHE_PROFILE} -- aws ecr get-login-password --region {DEFAULT_AWS_REGION}"
        )
    if uses_registry(SANDBOX_REGISTRY):
        pull_registries[SANDBOX_REGISTRY] = (
            f"aws-vault exec sso-agent-sandbox-account-admin-8h -- aws ecr get-login-password --region {DEFAULT_AWS_REGION}"
        )
    if pull_registries:
        flags["ddagent:imagePullRegistry"] = ",".join(pull_registries)
        flags["ddagent:imagePullUsername"] = ",".join("AWS" for _ in pull_registries)
        flags["ddagent:imagePullPassword"] = ",".join(
            ctx.run(command, hide=True).stdout.strip() for command in pull_registries.values()
        )
    return common_deploy(
        ctx,
        scenario_name,
        config_path,
        app_key_required,
        stack_name,
        pipeline_id,
        install_agent,
        install_installer,
        install_workload,
        agent_version,
        debug,
        flags,
        use_fakeintake,
        full_image_path,
        cluster_agent_full_image_path,
        agent_flavor,
        agent_config_path,
        agent_env,
        helm_config,
        local_package,
        pulumi_extra_args=pulumi_extra_args,
        pulumi_env=pulumi_env,
    )


def _check_key_pair(key_pair_to_search: str | None):
    if key_pair_to_search is None or key_pair_to_search == "":
        raise Exit("This scenario requires to define 'defaultKeyPairName' in the configuration file")
    output = subprocess.check_output(["ssh-add", "-L"])
    key_pairs: list[str] = []
    output = output.decode("utf-8")
    for line in output.splitlines():
        parts = line.split(" ")
        if parts:
            key_pair_path = os.path.basename(parts[-1])
            key_pair = os.path.splitext(key_pair_path)[0]
            key_pairs.append(key_pair)

    if key_pair_to_search not in key_pairs:
        raise Exit(
            f"Your key pair value '{key_pair_to_search}' is not find in ssh-agent. "
            + f"You may have issue to connect to the remote instance. Possible values are \n{key_pairs}. "
            + "You can skip this check by setting `checkKeyPair: false` in the config"
        )
