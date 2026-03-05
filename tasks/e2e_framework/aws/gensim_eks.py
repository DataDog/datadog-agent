import json
import os
from io import StringIO
from pathlib import Path

import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws.common import get_aws_wrapper
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.config import get_app_key
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/gensim-eks"

_DEFAULT_STACK_NAME = "gensim-eks"


@task(
    help={
        "episode": "Episode directory name under gensim-episodes/postmortems/ (e.g. authcore-pgbouncer-connection-pool-saturation). Omit for a cluster-only (M1) deploy.",
        "namespace": "Kubernetes namespace for the episode workloads (default: default)",
        "stack_name": doc.stack_name,
        "instance_type": "EC2 instance type for EKS worker nodes and build VM (default: t3.xlarge)",
        "config_path": doc.config_path,
        "debug": "Enable Pulumi debug logging",
    }
)
def create_gensim_eks(
    ctx: Context,
    episode: str | None = None,
    scenario: str | None = None,
    namespace: str = "default",
    stack_name: str = _DEFAULT_STACK_NAME,
    instance_type: str = "t3.xlarge",
    debug: bool = False,
    config_path: str | None = None,
) -> None:
    """
    Create an EKS cluster for running gensim episodes.

    Without --episode: provisions the cluster only (M1 mode, useful for debugging).
    With --episode: also provisions an EC2 build VM that builds episode service images
    and pushes them to ECR, then deploys the episode Helm chart (M2+).

    Images are built on EC2 rather than locally, which means:
      - No local Docker required
      - No cross-platform issues (build VM is x86_64, matching EKS nodes)
      - ECR auth is handled via the instance IAM role

    Examples:
        inv aws.eks.gensim.create
        inv aws.eks.gensim.create --episode=authcore-pgbouncer-connection-pool-saturation
    """
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config: {e}") from e

    extra_flags = {
        # Single Linux x86_64 node group. ARM, Bottlerocket, Windows, and GPU groups
        # are disabled to keep the cluster lean and start-up time short.
        "ddinfra:aws/defaultInstanceType": instance_type,
        "ddinfra:aws/eks/linuxNodeGroup": "true",
        "ddinfra:aws/eks/linuxARMNodeGroup": False,
        "ddinfra:aws/eks/linuxBottlerocketNodeGroup": False,
        "ddinfra:aws/eks/windowsNodeGroup": False,
        "ddinfra:aws/eks/gpuNodeGroup": False,
    }

    if episode is not None:
        extra_flags.update(_episode_flags(ctx, cfg, episode, namespace))
        if scenario is not None:
            extra_flags["gensim:scenario"] = scenario

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path=config_path,
        debug=debug,
        stack_name=stack_name,
        # install_agent=True sets ddagent:deploy=True and injects ddagent:apiKey,
        # which gates the M3 DD agent deployment in run.go via awsEnv.AgentDeploy().
        install_agent=episode is not None,
        install_workload=False,
        extra_flags=extra_flags,
    )

    _show_connection_message(ctx, full_stack_name, config_path)

    # M3+: post-deploy cleanup and monitoring instructions.
    if episode is not None:
        kubeconfig_path = f"{full_stack_name}-kubeconfig.yaml"
        try:
            local_config = config.get_local_config(config_path)
            aws_wrapper = get_aws_wrapper(local_config.get_aws().get_account())
        except Exception:
            aws_wrapper = "aws-vault exec sso-agent-sandbox-account-admin -- "

        # M3: remove stub agent
        _delete_stub_agent(ctx, kubeconfig_path, aws_wrapper, namespace)

        # M4: show runner monitoring instructions
        if scenario is not None:
            kube = f"KUBECONFIG={kubeconfig_path} {aws_wrapper}"
            tool.info("\n" + "=" * 70)
            tool.info("Episode runner started. Monitor progress:")
            tool.info(f"  {kube}kubectl logs -f job/gensim-runner -n {namespace}")
            tool.info("\nCheck pod status:")
            tool.info(f"  {kube}kubectl get pods -n {namespace}")
            tool.info("=" * 70)


@task(help={"stack_name": doc.stack_name})
def destroy_gensim_eks(
    ctx: Context,
    stack_name: str = _DEFAULT_STACK_NAME,
    config_path: str | None = None,
) -> None:
    """
    Destroy an EKS gensim cluster created with inv aws.eks.gensim.create.

    Example:
        inv aws.eks.gensim.destroy
        inv aws.eks.gensim.destroy --stack-name=my-gensim
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name, config_path=config_path)


# ── Helpers ───────────────────────────────────────────────────────────────────


def _get_gensim_repo_path() -> Path:
    """Locate the gensim-episodes/postmortems directory.

    Checks, in order:
      1. GENSIM_REPO_PATH environment variable
      2. Sibling directory: ../gensim-episodes/postmortems
      3. Go workspace: ~/go/src/github.com/DataDog/gensim-episodes/postmortems
    """
    env_path = os.getenv("GENSIM_REPO_PATH")
    if env_path:
        path = Path(env_path)
        if path.exists():
            return path

    current_dir = Path(__file__).parent
    repo_root = current_dir.parent.parent.parent  # up to datadog-agent root
    parent_dir = repo_root.parent

    candidates = [
        parent_dir / "gensim-episodes" / "postmortems",
        Path.home() / "go" / "src" / "github.com" / "DataDog" / "gensim-episodes" / "postmortems",
    ]
    for path in candidates:
        if path.exists():
            return path

    raise Exit("Could not find gensim-episodes repository. Set GENSIM_REPO_PATH environment variable.")


def _episode_flags(ctx: Context, cfg, episode: str, namespace: str) -> dict:
    """
    Validate the episode directory and return the Pulumi extra_flags needed for M2+.

    Image building and ECR pushing are handled by a build VM provisioned inside the
    Pulumi stack (run.go), not here. This function only needs to pass the episode
    location and the pre-computed ECR registry URL so Pulumi knows where to push.
    """
    gensim_path = _get_gensim_repo_path()
    episode_dir = gensim_path / episode
    chart_dir = episode_dir / "chart"

    if not episode_dir.exists():
        raise Exit(f"Episode directory not found: {episode_dir}")
    if not chart_dir.exists():
        raise Exit(f"Chart directory not found: {chart_dir}")

    aws_wrapper = get_aws_wrapper(cfg.get_aws().get_account())

    # Compute the ECR registry URL for this account/region.
    # The actual image build and push happens on an EC2 VM inside the Pulumi stack,
    # using the instance IAM role for auth — no local Docker or ECR credentials needed.
    ecr_registry = ""
    if (episode_dir / "docker-compose.yaml").exists():
        ecr_registry, _ = _get_ecr_registry(ctx, aws_wrapper)
        tool.info(f"ECR registry: {ecr_registry}")

    # Pass datadog-values.yaml path if it exists at the postmortems root.
    # Sets clusterName and kubelet.tlsVerify: false (required on EKS).
    datadog_values_path = gensim_path / "datadog-values.yaml"

    flags = {
        "gensim:episodeName": episode,
        "gensim:chartPath": str(chart_dir),
        "gensim:episodePath": str(episode_dir),
        "gensim:imageRegistry": ecr_registry,
        "gensim:namespace": namespace,
        # ddagent:appKey is not injected by deploy() without app_key_required=True.
        # apiKey is now injected by install_agent=True in the caller.
        "ddagent:appKey": get_app_key(cfg),
    }

    if datadog_values_path.exists():
        flags["gensim:datadogValuesPath"] = str(datadog_values_path)

    return flags


def _get_ecr_registry(ctx: Context, aws_wrapper: str) -> tuple[str, str]:
    """Return (ecr_registry_url, region) for the current AWS account."""
    account_buf = StringIO()
    ctx.run(
        f"{aws_wrapper}aws sts get-caller-identity --query Account --output text",
        out_stream=account_buf,
        hide="out",
    )
    account_id = account_buf.getvalue().strip()
    region = os.getenv("AWS_DEFAULT_REGION", "us-east-1")
    return f"{account_id}.dkr.ecr.{region}.amazonaws.com", region


def _delete_stub_agent(ctx: Context, kubeconfig_path: str, aws_wrapper: str, namespace: str) -> None:
    """Delete the episode chart's built-in stub datadog-agent after the real DaemonSet is deployed.

    The episode chart ships a basic Deployment-based agent that conflicts with the full
    DaemonSet-based agent deployed in M3. Running both would produce duplicate metrics.
    This runs after `pulumi up` so the kubeconfig is available on disk.
    """
    ctx.run(
        f"KUBECONFIG={kubeconfig_path} {aws_wrapper}kubectl delete "
        f"deployment/datadog-agent service/datadog-agent serviceaccount/datadog-agent "
        f"--ignore-not-found=true -n {namespace}",
        warn=True,
        hide="stderr",
    )
    tool.info("Stub datadog-agent removed (replaced by full DaemonSet-based agent).")


def _show_connection_message(ctx: Context, full_stack_name: str, config_path: str | None) -> None:
    """Write a local kubeconfig file and print kubectl + destroy commands."""
    from pydantic import ValidationError

    from tasks.e2e_framework import config

    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)

    kubeconfig_data = json.loads(outputs["dd-Cluster-gensim"]["kubeConfig"])
    kubeconfig_content = yaml.dump(kubeconfig_data)

    # Write with mode 0o600 — kubeconfigs contain credentials.
    kubeconfig_path = f"{full_stack_name}-kubeconfig.yaml"
    fd = os.open(kubeconfig_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with open(fd, "w") as f:
        f.write(kubeconfig_content)

    try:
        local_config = config.get_local_config(config_path)
        aws_wrapper = get_aws_wrapper(local_config.get_aws().get_account())
    except (ValidationError, Exit):
        aws_wrapper = "aws-vault exec sso-agent-sandbox-account-admin -- "

    short_stack = full_stack_name.split("/")[-1]

    tool.info(f"\nKubeconfig written to: {kubeconfig_path}")
    tool.info("\nTo connect to the cluster:")
    tool.info(f"  KUBECONFIG={kubeconfig_path} {aws_wrapper}kubectl get nodes")
    tool.info("\nTo destroy:")
    tool.info(f"  inv aws.eks.gensim.destroy --stack-name={short_stack}")
