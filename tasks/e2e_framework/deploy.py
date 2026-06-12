import json
import os
import sys
import tempfile
from typing import Any

import boto3
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo

from . import tool


def get_pipeline_commit_sha(pipeline_id: str) -> str | None:
    """
    Fetch the short (8-char) commit SHA associated with a GitLab pipeline.
    Returns None on failure.
    """
    try:
        token = os.environ.get('GITLAB_TOKEN')
        repo = get_gitlab_repo(token=token)
        pipeline = repo.pipelines.get(int(pipeline_id))
        return pipeline.sha[:8]
    except Exception as e:
        print(f"Warning: Could not fetch commit SHA for pipeline {pipeline_id}: {e}", file=sys.stderr)
        if 'GITLAB_TOKEN' not in os.environ:
            print(
                "No GITLAB_TOKEN environment variable found, set it with a GitLab Personal Access Token (read_api scope)",
                file=sys.stderr,
            )
        return None


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
    pulumi_extra_args: str = "",
    pulumi_env: dict[str, str] | None = None,
) -> str:
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    flags = extra_flags if extra_flags else {}

    if install_agent is None:
        install_agent = tool.get_default_agent_install()

    # Pulumi provisions infrastructure only — agent install is handled below
    # via e2e-install, which calls the Pulumi-free installer packages.
    flags["ddagent:deploy"] = False
    flags["ddupdater:deploy"] = install_installer

    if install_workload is None:
        install_workload = tool.get_default_workload_install()
    flags["ddtestworkload:deploy"] = install_workload

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}") from e

    flags["scenario"] = scenario_name
    flags["ddagent:fakeintake"] = use_fakeintake

    flags["ddagent:pipeline_id"] = "" if pipeline_id is None else pipeline_id

    # Configure SSH keys for the different cloud providers
    privateKeyPassword = cfg.get_aws().privateKeyPassword
    if privateKeyPassword is not None:
        flags["ddinfra:aws/defaultPrivateKeyPassword"] = privateKeyPassword

    privateKeyPassword = cfg.get_azure().privateKeyPassword
    if privateKeyPassword is not None:
        flags["ddinfra:az/defaultPrivateKeyPassword"] = privateKeyPassword

    privateKeyPassword = cfg.get_gcp().privateKeyPassword
    if privateKeyPassword is not None:
        flags["ddinfra:gcp/defaultPrivateKeyPassword"] = privateKeyPassword

    privateKeyPath = cfg.get_azure().privateKeyPath
    if privateKeyPath is not None:
        flags["ddinfra:az/defaultPrivateKeyPath"] = privateKeyPath

    privateKeyPath = cfg.get_gcp().privateKeyPath
    if privateKeyPath is not None:
        flags["ddinfra:gcp/defaultPrivateKeyPath"] = privateKeyPath

    privateKeyPath = cfg.get_aws().privateKeyPath
    if privateKeyPath is not None:
        flags["ddinfra:aws/defaultPrivateKeyPath"] = privateKeyPath

    publicKeyPath = cfg.get_aws().publicKeyPath
    if publicKeyPath is not None:
        flags["ddinfra:aws/defaultPublicKeyPath"] = publicKeyPath

    publicKeyPath = cfg.get_azure().publicKeyPath
    if publicKeyPath is not None:
        flags["ddinfra:az/defaultPublicKeyPath"] = publicKeyPath

    publicKeyPath = cfg.get_gcp().publicKeyPath
    if publicKeyPath is not None:
        flags["ddinfra:gcp/defaultPublicKeyPath"] = publicKeyPath

    if pipeline_id:
        commit_sha = get_pipeline_commit_sha(pipeline_id)
        if commit_sha:
            flags["ddagent:commit_sha"] = commit_sha

    # When using fakeintake, enable dual shipping to send data to both fakeintake and Datadog.
    if use_fakeintake is True:
        flags["ddagent:dualshipping"] = True

    # add stack params values
    stackParams = cfg.get_stack_params()
    for namespace in stackParams:
        for key, value in stackParams[namespace].items():
            flags[f"{namespace}:{key}"] = value

    if app_key_required:
        flags["ddagent:appKey"] = config.get_app_key(cfg)

    full_stack_name = _deploy(
        ctx,
        stack_name,
        flags,
        debug,
        cfg.get_pulumi().logLevel,
        cfg.get_pulumi().logToStdErr,
        pulumi_extra_args=pulumi_extra_args,
        pulumi_env=pulumi_env,
    )

    # Install the agent via e2e-install (separate from Pulumi).
    if install_agent and not install_installer:
        _install_agent_via_cli(
            ctx=ctx,
            full_stack_name=full_stack_name,
            scenario_name=scenario_name,
            pipeline_id=pipeline_id,
            agent_version=agent_version,
            agent_flavor=agent_flavor,
            agent_config_path=agent_config_path,
            helm_config=helm_config,
            full_image_path=full_image_path,
        )

    return full_stack_name


def _install_agent_via_cli(
    ctx: Context,
    full_stack_name: str,
    scenario_name: str,
    pipeline_id: str | None,
    agent_version: str | None,
    agent_flavor: str | None,
    agent_config_path: str | None,
    helm_config: str | None,
    full_image_path: str | None,
) -> None:
    """Run e2e-install to install the agent on the already-provisioned env.

    Retrieves the Pulumi stack outputs, maps them to an envdesc.Descriptor,
    builds an installspec.Spec from the agent parameters, and invokes the
    e2e-install binary.
    """
    # Get Pulumi stack outputs (the provisioned env's connection info).
    pulumi_outputs = tool.get_stack_json_outputs(ctx, full_stack_name)

    # Determine cloud and env type from scenario name.
    cloud = _cloud_from_scenario(scenario_name)
    env_type = _env_type_from_scenario(scenario_name)

    # Build an env descriptor by heuristically mapping Pulumi output keys to
    # the field names that envdesc.LoadEnv expects.
    env_descriptor = {
        "scenario": scenario_name,
        "env_type": env_type,
        "resources": _map_pulumi_outputs_to_fields(pulumi_outputs),
    }

    # Build an install spec from the agent parameters.
    spec: dict[str, Any] = {"env_type": env_type, "cloud": cloud}

    if env_type in ("host", "windowshost"):
        version: dict[str, str] = {}
        if pipeline_id:
            version["pipeline_id"] = pipeline_id
        if agent_version:
            parts = agent_version.split(".", 1)
            if len(parts) == 2:
                version["major"], version["minor"] = parts
            else:
                version["major"] = parts[0]
        if agent_flavor:
            version["flavor"] = agent_flavor
        host_spec: dict[str, Any] = {"version": version}
        if agent_config_path:
            try:
                with open(agent_config_path) as f:
                    host_spec["agent_config"] = f.read()
            except OSError:
                pass
        spec["host"] = host_spec

    elif env_type == "kubernetes":
        k8s_spec: dict[str, Any] = {}
        if helm_config:
            k8s_spec["helm_values"] = [helm_config]
        spec["kubernetes"] = k8s_spec

    elif env_type == "dockerhost":
        docker_spec: dict[str, Any] = {}
        if full_image_path:
            docker_spec["full_image_path"] = full_image_path
        if agent_flavor and "fips" in agent_flavor:
            docker_spec["fips"] = True
        spec["docker"] = docker_spec

    # Write descriptor and spec to temp files and invoke e2e-install.
    with tempfile.TemporaryDirectory() as tmpdir:
        env_file = os.path.join(tmpdir, "env.json")
        spec_file = os.path.join(tmpdir, "spec.json")

        with open(env_file, "w") as f:
            json.dump(env_descriptor, f, indent=2)
        with open(spec_file, "w") as f:
            json.dump(spec, f, indent=2)

        e2e_install_bin = _find_e2e_install_binary()
        ctx.run(f"{e2e_install_bin} --env {env_file} --spec {spec_file}")


def _find_e2e_install_binary() -> str:
    """Return the path to the e2e-install binary, building it if necessary."""
    # Check if already built
    candidate = os.path.join(
        os.path.dirname(os.path.dirname(os.path.dirname(__file__))),
        "test",
        "e2e-framework",
        "bin",
        "e2e-install",
    )
    if os.path.exists(candidate):
        return candidate

    # Build it
    framework_dir = os.path.join(
        os.path.dirname(os.path.dirname(os.path.dirname(__file__))),
        "test",
        "e2e-framework",
    )
    bin_dir = os.path.join(framework_dir, "bin")
    os.makedirs(bin_dir, exist_ok=True)
    import subprocess

    subprocess.check_call(
        ["go", "build", "-o", os.path.join(bin_dir, "e2e-install"), "./cmd/e2e-install/"],
        cwd=framework_dir,
    )
    return os.path.join(bin_dir, "e2e-install")


def _cloud_from_scenario(scenario: str) -> str:
    """Map a scenario name to a cloud string for the install spec."""
    if scenario.startswith("az/") or scenario.startswith("azure/"):
        return "az"
    if scenario.startswith("gcp/"):
        return "gcp"
    return "aws"


def _env_type_from_scenario(scenario: str) -> str:
    """Map a scenario name to an envdesc env_type."""
    if any(x in scenario for x in ("eks", "gke", "aks", "kind", "openshift")):
        return "kubernetes"
    if "docker" in scenario:
        return "dockerhost"
    if "ecs" in scenario:
        return "ecs"
    return "host"


def _map_pulumi_outputs_to_fields(pulumi_outputs: dict) -> dict:
    """Heuristically map Pulumi stack output values to envdesc field names.

    Pulumi export keys are stack-name-specific (e.g. "dd-Host-aws-vm").
    envdesc.LoadEnv looks up by field name ("RemoteHost", "FakeIntake", ...).
    We identify each component by a distinctive JSON key in its output struct.
    """
    result: dict[str, Any] = {}
    used: set[str] = set()

    for raw_value in pulumi_outputs.values():
        if not isinstance(raw_value, dict):
            continue
        keys = set(raw_value.keys())

        if "address" in keys and "username" in keys and "RemoteHost" not in used:
            result["RemoteHost"] = raw_value
            used.add("RemoteHost")
        elif "kubeConfig" in keys and "KubernetesCluster" not in used:
            result["KubernetesCluster"] = raw_value
            used.add("KubernetesCluster")
        elif "url" in keys and "host" in keys and "port" in keys and "FakeIntake" not in used:
            result["FakeIntake"] = raw_value
            used.add("FakeIntake")
        elif "dockerManager" in keys and "Docker" not in used:
            result["Docker"] = raw_value
            used.add("Docker")

    return result


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
    pulumi_extra_args: str = "",
    pulumi_env: dict[str, str] | None = None,
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
    extra = f" {pulumi_extra_args}" if pulumi_extra_args else ""
    env_prefix = " ".join(f"{k}={v}" for k, v in (pulumi_env or {}).items())
    env_prefix = f"{env_prefix} " if env_prefix else ""
    cmd = f"{env_prefix}pulumi {global_flags} up --yes{extra} -s {stack_name} {up_flags}"

    pty = not pulumi_extra_args  # disable pty when extra args are set (e.g. --non-interactive)
    if tool.is_windows():
        pty = False
    ctx.run(cmd, pty=pty)
    return stack_name
