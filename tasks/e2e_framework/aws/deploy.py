import os
import subprocess
from typing import Any

from invoke.context import Context
from invoke.exceptions import Exit

from tasks.e2e_framework import tool
from tasks.e2e_framework.deploy import deploy as common_deploy

default_public_path_key_name = "ddinfra:aws/defaultPublicKeyPath"
default_private_path_key_name = "ddinfra:aws/defaultPrivateKeyPath"


def deploy(
    ctx: Context,
    scenario_name: str,
    config_path: str | None = None,
    key_pair_required: bool = False,
    public_key_required: bool = False,
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
) -> str:
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    flags = extra_flags if extra_flags else {}

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}") from e

    defaultPublicKeyPath = cfg.get_aws().publicKeyPath
    if public_key_required and defaultPublicKeyPath is None:
        raise Exit(f"Your scenario requires to define {default_public_path_key_name} in the configuration file")
    flags[default_public_path_key_name] = defaultPublicKeyPath

    privateKeyPath = cfg.get_aws().privateKeyPath
    if privateKeyPath is not None:
        flags[default_private_path_key_name] = privateKeyPath

    awsKeyPairName = cfg.get_aws().keyPairName

    flags["ddinfra:aws/defaultKeyPairName"] = awsKeyPairName
    aws_account = cfg.get_aws().get_account()
    flags.setdefault("ddinfra:env", "aws/" + aws_account)

    # Verify image deployed and not outdated in s3
    if deploy_job is not None and pipeline_id is not None:
        cmd = f"inv -e check-s3-image-exists --pipeline-id={pipeline_id} --deploy-job={deploy_job}"
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

    if (
        full_image_path is not None
        and full_image_path.startswith("669783387624.dkr.ecr.us-east-1.amazonaws.com/")
        or cluster_agent_full_image_path is not None
        and cluster_agent_full_image_path.startswith("669783387624.dkr.ecr.us-east-1.amazonaws.com/")
    ):
        flags["ddagent:imagePullRegistry"] = "669783387624.dkr.ecr.us-east-1.amazonaws.com"
        flags["ddagent:imagePullUsername"] = "AWS"
        flags["ddagent:imagePullPassword"] = ctx.run(
            "aws-vault exec sso-agent-qa-read-only -- aws ecr get-login-password --region us-east-1", hide=True
        ).stdout.strip()
    elif (
        full_image_path is not None
        and full_image_path.startswith("376334461865.dkr.ecr.us-east-1.amazonaws.com/")
        or cluster_agent_full_image_path is not None
        and cluster_agent_full_image_path.startswith("376334461865.dkr.ecr.us-east-1.amazonaws.com/")
    ):
        flags["ddagent:imagePullRegistry"] = "376334461865.dkr.ecr.us-east-1.amazonaws.com"
        flags["ddagent:imagePullUsername"] = "AWS"
        flags["ddagent:imagePullPassword"] = ctx.run(
            "aws-vault exec sso-agent-sandbox-account-admin -- aws ecr get-login-password --region us-east-1",
            hide=True,
        ).stdout.strip()
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
