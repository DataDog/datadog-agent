import glob
import json
import os
from datetime import datetime, timezone
from io import StringIO
from pathlib import Path

import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws.common import get_aws_wrapper
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/gensim-eks"

_DEFAULT_STACK_NAME = "gensim-eks"


def _get_api_key(cfg):
    from tasks.e2e_framework.config import get_api_key

    return get_api_key(cfg)


def _get_app_key(cfg):
    from tasks.e2e_framework.config import get_app_key

    return get_app_key(cfg)


@task(
    help={
        "image": "Full agent Docker image path (e.g. docker.io/datadog/agent-dev:my-tag)",
        "episodes": "Comma-separated episode:scenario pairs (e.g. authcore-pgbouncer:pool-saturation,ep2:scen-a)",
        "s3_bucket": "S3 bucket for parquet results upload",
        "namespace": "Kubernetes namespace for the episode workloads (default: default)",
        "stack_name": doc.stack_name,
        "instance_type": "EC2 instance type for EKS worker nodes (default: t3.xlarge)",
        "config_path": doc.config_path,
        "debug": "Enable Pulumi debug logging",
    }
)
def submit_gensim_eks(
    ctx: Context,
    image: str = "",
    episodes: str = "",
    s3_bucket: str | None = None,
    namespace: str = "default",
    stack_name: str = _DEFAULT_STACK_NAME,
    instance_type: str = "t3.xlarge",
    debug: bool = False,
    config_path: str | None = None,
) -> None:
    """
    Submit a gensim evaluation run to an EKS cluster.

    Parses the --episodes flag, validates episode directories, ensures the cluster
    is not busy, then deploys via Pulumi. The orchestrator Job on the cluster handles
    agent installation and episode sequencing.

    Examples:
        inv aws.eks.gensim.submit --image=docker.io/datadog/agent-dev:my-tag --episodes=authcore-pgbouncer:pool-saturation
        inv aws.eks.gensim.submit --image=docker.io/datadog/agent-dev:my-tag --episodes=ep1:scen-a,ep2:scen-b --s3-bucket=my-bucket
    """
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    if not image:
        raise Exit("--image is required (e.g. docker.io/datadog/agent-dev:my-tag)")
    if not episodes:
        raise Exit("--episodes is required (e.g. authcore-pgbouncer:pool-saturation,ep2:scen-a)")

    # ── 1. Parse and validate episode:scenario pairs ──────────────────────
    episode_pairs = []
    for pair in episodes.split(","):
        pair = pair.strip()
        if not pair:
            continue
        parts = pair.split(":")
        if len(parts) != 2 or not parts[0] or not parts[1]:
            raise Exit(f"Invalid episode:scenario format: '{pair}'. Expected 'episode:scenario'.")
        episode_pairs.append((parts[0], parts[1]))

    if not episode_pairs:
        raise Exit("No valid episode:scenario pairs found in --episodes.")

    # ── 2. Validate episode directories ───────────────────────────────────
    gensim_repo_path = _get_gensim_repo_path()
    for ep_name, scen_name in episode_pairs:
        ep_dir = _find_episode_dir(gensim_repo_path, ep_name)
        chart_dir = ep_dir / "chart"
        scenario_file = ep_dir / "episodes" / f"{scen_name}.yaml"

        if not chart_dir.exists():
            raise Exit(f"Chart directory not found: {chart_dir}")
        if not scenario_file.exists():
            raise Exit(f"Scenario file not found: {scenario_file}")

    # ── 3. Capture gensim-episodes git SHA ────────────────────────────────
    sha_buf = StringIO()
    ctx.run(
        f"git -C {gensim_repo_path} rev-parse --short=10 HEAD",
        out_stream=sha_buf,
        hide="out",
    )
    gensim_sha = sha_buf.getvalue().strip()

    # ── 4. Validate clean checkout ────────────────────────────────────────
    status_buf = StringIO()
    ctx.run(
        f"git -C {gensim_repo_path} status --porcelain",
        out_stream=status_buf,
        hide="out",
    )
    if status_buf.getvalue().strip():
        raise Exit(
            f"gensim-episodes checkout is dirty ({gensim_repo_path}).\n" "Commit or stash changes before submitting."
        )

    # ── 5-6. Check cluster accessibility and guard against busy cluster ──
    try:
        local_config = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config: {e}") from e

    try:
        aws_wrapper = get_aws_wrapper(local_config.get_aws().get_account())
    except Exception:
        aws_wrapper = "aws-vault exec sso-agent-sandbox-account-admin -- "

    kubeconfig_path = _find_kubeconfig(stack_name)
    if os.path.exists(kubeconfig_path):
        # Check if cluster is reachable
        cluster_ok = ctx.run(
            f"KUBECONFIG={kubeconfig_path} kubectl cluster-info",
            warn=True,
            hide=True,
        )
        if cluster_ok and cluster_ok.ok:
            # Check for active orchestrator job
            job_result = ctx.run(
                f"KUBECONFIG={kubeconfig_path} kubectl get job gensim-orchestrator " f"-n {namespace} -o json",
                warn=True,
                hide=True,
            )
            if job_result and job_result.ok:
                job_data = json.loads(job_result.stdout)
                status = job_data.get("status", {})
                if status.get("active", 0) == 1:
                    # Show current run status if available
                    cm_result = ctx.run(
                        f"KUBECONFIG={kubeconfig_path} kubectl get configmap "
                        f"gensim-run-status -n {namespace} -o jsonpath='{{.data.status}}'",
                        warn=True,
                        hide=True,
                    )
                    if cm_result and cm_result.ok and cm_result.stdout.strip():
                        tool.info("Current run status:")
                        tool.info(cm_result.stdout.strip())
                    raise Exit("Cluster busy. Retry after the current run completes.")
                else:
                    # Job completed or failed -- clean it up
                    tool.info("Cleaning up previous orchestrator job.")
                    ctx.run(
                        f"KUBECONFIG={kubeconfig_path} kubectl delete job " f"gensim-orchestrator -n {namespace}",
                        warn=True,
                        hide=True,
                    )

    # ── 7. Compute ECR registry URL if any episode has docker-compose.yaml
    ecr_registry = ""
    for ep_name, _ in episode_pairs:
        ep_dir = _find_episode_dir(gensim_repo_path, ep_name)
        if (ep_dir / "docker-compose.yaml").exists():
            ecr_registry, _ = _get_ecr_registry(ctx, aws_wrapper)
            tool.info(f"ECR registry: {ecr_registry}")
            break

    # ── 8. Deploy via Pulumi ──────────────────────────────────────────────
    extra_flags = {
        # Cluster shape
        "ddinfra:aws/defaultInstanceType": instance_type,
        "ddinfra:aws/eks/linuxNodeGroup": "true",
        "ddinfra:aws/eks/linuxARMNodeGroup": False,
        "ddinfra:aws/eks/linuxBottlerocketNodeGroup": False,
        "ddinfra:aws/eks/windowsNodeGroup": False,
        "ddinfra:aws/eks/gpuNodeGroup": False,
        # Gensim orchestrator config
        "gensim:episodes": episodes,
        "gensim:agentImage": image,
        "gensim:gensimSha": gensim_sha,
        "gensim:s3Bucket": s3_bucket or "",
        "gensim:namespace": namespace,
        "gensim:imageRegistry": ecr_registry,
        "gensim:episodeDataDir": str(gensim_repo_path),
        # Datadog keys -- must be explicit since install_agent=False
        "ddagent:apiKey": _get_api_key(local_config),
        "ddagent:appKey": _get_app_key(local_config),
    }

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path=config_path,
        debug=debug,
        stack_name=stack_name,
        install_agent=False,
        install_workload=False,
        extra_flags=extra_flags,
    )

    # ── 9. Show connection message ────────────────────────────────────────
    _show_connection_message(ctx, full_stack_name, config_path)

    # ── 10-11. Print monitoring instructions ──────────────────────────────
    run_id = f"eval-{datetime.now(timezone.utc).strftime('%Y%m%d')}-{gensim_sha[:7]}"
    kube = f"KUBECONFIG={full_stack_name}-kubeconfig.yaml {aws_wrapper}"

    tool.info("\n" + "=" * 70)
    tool.info(f"Run submitted: {run_id}")
    tool.info(f"  Image:    {image}")
    tool.info(f"  Episodes: {episodes}")
    tool.info(f"  SHA:      {gensim_sha}")
    tool.info("\nMonitor orchestrator:")
    tool.info(f"  {kube}kubectl logs -f job/gensim-orchestrator -n {namespace}")
    tool.info("\nCheck run status:")
    tool.info("  inv aws.eks.gensim.status")
    tool.info("\nPod status:")
    tool.info(f"  {kube}kubectl get pods -n {namespace}")
    tool.info("=" * 70)


@task(
    help={
        "stack_name": doc.stack_name,
        "config_path": doc.config_path,
        "namespace": "Kubernetes namespace (default: default)",
    }
)
def status_gensim_eks(
    ctx: Context,
    stack_name: str = _DEFAULT_STACK_NAME,
    config_path: str | None = None,
    namespace: str = "default",
) -> None:
    """
    Show the status of a gensim evaluation run.

    Reads the gensim-run-status ConfigMap from the cluster and displays
    progress for each episode:scenario pair.

    Example:
        inv aws.eks.gensim.status
    """

    # ── 1. Get kubeconfig and aws_wrapper ─────────────────────────────────
    kubeconfig_path = _find_kubeconfig(stack_name)
    if not os.path.exists(kubeconfig_path):
        tool.warn("No gensim cluster found. Run `inv aws.eks.gensim.submit` first.")
        return

    # ── 2. Read the status ConfigMap ──────────────────────────────────────
    # No aws_wrapper needed -- kubeconfig has embedded auth credentials.
    result = ctx.run(
        f"KUBECONFIG={kubeconfig_path} kubectl get configmap "
        f"gensim-run-status -n {namespace} -o jsonpath='{{.data.status}}'",
        warn=True,
        hide=True,
    )

    if not result or not result.ok or not result.stdout.strip():
        tool.info("No active or recent evaluation run.")
        return

    # ── 3. Parse and render status ────────────────────────────────────────
    try:
        status_data = json.loads(result.stdout.strip())
    except json.JSONDecodeError:
        tool.warn(f"Could not parse status ConfigMap: {result.stdout.strip()}")
        return

    run_id = status_data.get("runId", "unknown")
    agent_image = status_data.get("image", "unknown")
    gensim_sha = status_data.get("gensimSha", "unknown")

    tool.info(f"Run: {run_id}")
    tool.info(f"Image: {agent_image}")
    tool.info(f"Gensim SHA: {gensim_sha}")
    tool.info("")

    for ep in status_data.get("episodes", []):
        ep_name = ep.get("episode", "?")
        scen_name = ep.get("scenario", "?")
        state = ep.get("status", "unknown")
        phase = ep.get("phase", "")
        duration_secs = ep.get("durationSeconds", 0)
        parquet_count = ep.get("parquetFiles", 0)

        tag = f"[{state}]"
        line = f"  {tag:<12} {ep_name} / {scen_name}"
        if state == "done" and duration_secs:
            mins = duration_secs // 60
            line += f"  ({mins}m, {parquet_count} parquet)"
        elif phase:
            line += f"  ({phase})"
        tool.info(line)


@task(
    help={
        "stack_name": doc.stack_name,
        "config_path": doc.config_path,
    }
)
def destroy_gensim_eks(
    ctx: Context,
    stack_name: str = _DEFAULT_STACK_NAME,
    config_path: str | None = None,
) -> None:
    """
    Destroy an EKS gensim cluster created with inv aws.eks.gensim.submit.

    Example:
        inv aws.eks.gensim.destroy
        inv aws.eks.gensim.destroy --stack-name=my-gensim
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name, config_path=config_path)


# -- Helpers -------------------------------------------------------------------


def _find_kubeconfig(stack_name: str) -> str:
    """Find the kubeconfig file for the given stack name.

    The file is written by _show_connection_message as {full_stack_name}-kubeconfig.yaml
    where full_stack_name includes the user prefix (e.g. scott-opell-gensim-eks).
    """
    matches = glob.glob(f"*{stack_name}-kubeconfig.yaml")
    if matches:
        return matches[0]
    return f"{stack_name}-kubeconfig.yaml"


def _get_gensim_repo_path() -> Path:
    """Locate the gensim-episodes repository root.

    Checks, in order:
      1. GENSIM_REPO_PATH environment variable
      2. Sibling directory: ../gensim-episodes
      3. Go workspace: ~/go/src/github.com/DataDog/gensim-episodes
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
        parent_dir / "gensim-episodes",
        Path.home() / "go" / "src" / "github.com" / "DataDog" / "gensim-episodes",
    ]
    for path in candidates:
        if path.exists():
            return path

    raise Exit("Could not find gensim-episodes repository. Set GENSIM_REPO_PATH environment variable.")


# Episode subdirectories to search within the gensim-episodes repo.
_EPISODE_SUBDIRS = ["postmortems", "synthetics"]


def _find_episode_dir(repo_path: Path, ep_name: str) -> Path:
    """Find an episode directory by searching known subdirectories.

    Also supports legacy GENSIM_REPO_PATH pointing directly at a subdirectory
    (e.g. .../postmortems) by checking for a direct child match first.
    """
    # Direct child (legacy: GENSIM_REPO_PATH=.../postmortems)
    direct = repo_path / ep_name
    if direct.exists():
        return direct
    # Search known subdirectories
    for subdir in _EPISODE_SUBDIRS:
        candidate = repo_path / subdir / ep_name
        if candidate.exists():
            return candidate
    searched = ", ".join(f"{subdir}/" for subdir in _EPISODE_SUBDIRS)
    raise Exit(f"Episode '{ep_name}' not found. Searched: {repo_path} and {searched} under {repo_path}")


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


def _show_connection_message(ctx: Context, full_stack_name: str, config_path: str | None) -> None:
    """Write a local kubeconfig file and print kubectl + destroy commands."""
    from pydantic import ValidationError

    from tasks.e2e_framework import config

    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)

    kubeconfig_data = json.loads(outputs["dd-Cluster-gensim"]["kubeConfig"])
    kubeconfig_content = yaml.dump(kubeconfig_data)

    # Write with mode 0o600 -- kubeconfigs contain credentials.
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
