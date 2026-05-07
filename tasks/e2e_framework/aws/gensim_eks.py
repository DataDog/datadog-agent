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


# Observer modes for the gensim agent:
#   record-parquet          - Record observer data to parquet files for offline testbench replay
#   live-anomaly-detection  - Run live edge anomaly detection, send events to Datadog
#   live-and-record         - Both simultaneously (for A/B comparison of testbench vs live)
_VALID_MODES = ("record-parquet", "live-anomaly-detection", "live-and-record")


@task(
    help={
        "image": "Full agent Docker image path (e.g. docker.io/datadog/agent-dev:my-tag)",
        "episodes": "Comma-separated episode:scenario pairs (e.g. authcore-pgbouncer:pool-saturation,ep2:scen-a)",
        "episode_manifest": "Path to a JSON manifest listing episode/scenario pairs (e.g. ./gensim-eval-scenarios.json)",
        "s3_bucket": "S3 bucket for parquet results upload",
        "namespace": "Kubernetes namespace for the episode workloads (default: default)",
        "stack_name": doc.stack_name,
        "instance_type": "EC2 instance type for EKS worker nodes (default: t3.xlarge)",
        "mode": f"Observer mode: {' or '.join(_VALID_MODES)} (default: record-parquet)",
        "config_path": doc.config_path,
        "debug": "Enable Pulumi debug logging",
        "skip_build": "Skip episode image building (use cached ECR images from a previous run)",
    }
)
def submit_gensim_eks(
    ctx: Context,
    image: str = "",
    episodes: str = "",
    episode_manifest: str = "",
    s3_bucket: str | None = None,
    namespace: str = "default",
    stack_name: str = _DEFAULT_STACK_NAME,
    instance_type: str = "t3.xlarge",
    mode: str = "record-parquet",
    debug: bool = False,
    config_path: str | None = None,
    skip_build: bool = False,
) -> None:
    """
    Submit a gensim evaluation run to an EKS cluster.

    Parses the --episodes flag (or --episode-manifest), validates episode directories,
    ensures the cluster is not busy, then deploys via Pulumi. The orchestrator Job on
    the cluster handles agent installation and episode sequencing.

    Modes:
        record-parquet          - Collect observer data to parquet files for offline analysis
        live-anomaly-detection  - Run live edge anomaly detection, sending events to Datadog

    Examples:
        inv aws.eks.gensim.submit --image=docker.io/datadog/agent-dev:my-tag --episodes=authcore-pgbouncer:pool-saturation
        inv aws.eks.gensim.submit --image=... --episode-manifest=./gensim-eval-scenarios.json
        inv aws.eks.gensim.submit --image=... --episodes=... --mode=live-anomaly-detection
    """
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    if not image:
        raise Exit("--image is required (e.g. docker.io/datadog/agent-dev:my-tag)")
    if episodes and episode_manifest:
        raise Exit("--episodes and --episode-manifest are mutually exclusive.")
    if not episodes and not episode_manifest:
        raise Exit("One of --episodes or --episode-manifest is required.")
    if mode not in _VALID_MODES:
        raise Exit(f"--mode must be one of {_VALID_MODES}, got '{mode}'")

    # ── 1. Parse and validate episode:scenario pairs ──────────────────────
    episode_pairs = []
    if episode_manifest:
        manifest_path = Path(episode_manifest)
        if not manifest_path.exists():
            raise Exit(f"Episode manifest not found: {episode_manifest}")
        try:
            entries = json.loads(manifest_path.read_text())
        except json.JSONDecodeError as e:
            raise Exit(f"Failed to parse episode manifest: {e}") from e
        if not isinstance(entries, list):
            raise Exit(f"Episode manifest must be a JSON array, got {type(entries).__name__}.")
        for entry in entries:
            ep_name = entry.get("episode", "").strip()
            scen_name = entry.get("scenario", "").strip()
            pinned_sha = entry.get("sha", "").strip()
            if not ep_name or not scen_name:
                raise Exit(f"Invalid manifest entry (missing episode or scenario): {entry}")
            episode_pairs.append((ep_name, scen_name, pinned_sha))
        # Reconstruct the episodes string for downstream use (Pulumi config, logging)
        episodes = ",".join(f"{ep}:{sc}" for ep, sc, _ in episode_pairs)
    else:
        for pair in episodes.split(","):
            pair = pair.strip()
            if not pair:
                continue
            parts = pair.split(":")
            if len(parts) != 2 or not parts[0] or not parts[1]:
                raise Exit(f"Invalid episode:scenario format: '{pair}'. Expected 'episode:scenario'.")
            episode_pairs.append((parts[0], parts[1], ""))

    if not episode_pairs:
        raise Exit("No valid episode:scenario pairs found.")

    # ── 2. Validate episode directories and pinned SHAs ───────────────────
    gensim_repo_path = _get_gensim_repo_path()
    changed_episodes = []
    for ep_name, scen_name, pinned_sha in episode_pairs:
        ep_dir = _find_episode_dir(gensim_repo_path, ep_name)
        chart_dir = ep_dir / "chart"
        scenario_file = ep_dir / "episodes" / f"{scen_name}.yaml"

        if not chart_dir.exists():
            raise Exit(f"Chart directory not found: {chart_dir}")
        if not scenario_file.exists():
            raise Exit(f"Scenario file not found: {scenario_file}")

        if pinned_sha:
            rel_path = ep_dir.relative_to(gensim_repo_path)
            sha_buf = StringIO()
            result = ctx.run(
                f"git -C {gensim_repo_path} rev-parse HEAD:{rel_path}",
                out_stream=sha_buf,
                hide="out",
                warn=True,
            )
            if not result.ok:
                tool.warn(f"Could not resolve SHA for '{ep_name}' (directory may have moved). Skipping SHA check.")
            else:
                actual_sha = sha_buf.getvalue().strip()
                if actual_sha != pinned_sha:
                    changed_episodes.append((ep_name, pinned_sha, actual_sha, rel_path))

    if changed_episodes:
        tool.warn("\nThe following episodes have changed since the manifest was pinned:")
        for ep_name, pinned_sha, actual_sha, _ in changed_episodes:
            tool.warn(f"\n  {ep_name}")
            diff_buf = StringIO()
            ctx.run(
                f"git -C {gensim_repo_path} diff-tree -r {pinned_sha} {actual_sha}",
                out_stream=diff_buf,
                hide="out",
                warn=True,
            )
            for line in diff_buf.getvalue().strip().splitlines():
                # Format: :<old_mode> <new_mode> <old_sha> <new_sha> <status>\t<path>
                parts = line.split("\t", 1)
                if len(parts) == 2:
                    tool.warn(f"    modified: {parts[1]}")
        tool.warn(
            "\nResults may not be comparable to previous runs. "
            "Run `inv aws.eks.gensim.update-manifest-shas` after a successful run to re-pin."
        )
        answer = input("\nContinue anyway? [y/N]: ").strip().lower()
        if answer != "y":
            raise Exit("Aborted.")

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
    if skip_build:
        tool.info("Skipping episode image build (--skip-build). Using cached ECR images.")
    else:
        for ep_name, _, _sha in episode_pairs:
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
        "gensim:mode": mode,
        # Datadog keys -- must be explicit since install_agent=False
        "ddagent:apiKey": _get_api_key(local_config),
        "ddagent:appKey": _get_app_key(local_config),
    }

    # Gensim-specific Pulumi flags:
    # --refresh: reconcile state with cluster reality (resources may be deleted by infra cleaner or stop-all)
    # --non-interactive: capture error output instead of swallowing it in the TUI
    # PULUMI_K8S_DELETE_UNREACHABLE: purge stale k8s resources when cluster is gone
    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path=config_path,
        debug=debug,
        stack_name=stack_name,
        install_agent=False,
        install_workload=False,
        extra_flags=extra_flags,
        pulumi_extra_args="--refresh --non-interactive",
        pulumi_env={
            "PULUMI_SKIP_UPDATE_CHECK": "1",
            "PULUMI_K8S_DELETE_UNREACHABLE": "true",
        },
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
        "namespace": "Kubernetes namespace (default: default)",
    }
)
def stop_all_gensim_eks(
    ctx: Context,
    stack_name: str = _DEFAULT_STACK_NAME,
    namespace: str = "default",
) -> None:
    """
    Stop all gensim workloads without destroying the EKS cluster.

    Kills the orchestrator job, uninstalls all helm releases, deletes
    remaining pods, and clears the run status configmap. The cluster
    stays up for fast resubmission.

    Example:
        inv aws.eks.gensim.stop-all
    """
    kubeconfig_path = _find_kubeconfig(stack_name)
    if not os.path.exists(kubeconfig_path):
        tool.warn("No gensim cluster found.")
        return

    kube = f"KUBECONFIG={kubeconfig_path}"

    # 1. Kill orchestrator job
    tool.info("Deleting orchestrator job...")
    ctx.run(
        f"{kube} kubectl delete job gensim-orchestrator -n {namespace} --force --grace-period=0", warn=True, hide=True
    )

    # 2. Uninstall all helm releases cleanly
    tool.info("Uninstalling helm releases...")
    result = ctx.run(f"{kube} helm ls -n {namespace} -a -q", warn=True, hide=True)
    if result and result.ok and result.stdout.strip():
        for release in result.stdout.strip().splitlines():
            release = release.strip()
            if release:
                tool.info(f"  helm uninstall {release}")
                ctx.run(f"{kube} helm uninstall {release} -n {namespace} --wait", warn=True, hide=True)

    # 3. Clean up orphaned helm secrets (in case helm uninstall missed any)
    ctx.run(f"{kube} kubectl delete secrets -l owner=helm -n {namespace}", warn=True, hide=True)

    # 4. Delete all workload resources (Deployments, DaemonSets, Services, etc.)
    tool.info("Deleting workload resources...")
    ctx.run(
        f"{kube} kubectl delete deployment,daemonset,statefulset,service,configmap,secret,job"
        f" --all -n {namespace} --force --grace-period=0",
        warn=True,
        hide=True,
    )

    # 5. Delete remaining pods
    tool.info("Deleting remaining pods...")
    ctx.run(f"{kube} kubectl delete pods --all -n {namespace} --force --grace-period=0", warn=True, hide=True)

    tool.info("Cluster cleaned. Ready for next submit.")


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


_EVAL_MANIFEST_PATH = Path(__file__).parent.parent.parent.parent / "q_branch" / "gensim-eval-scenarios.json"


@task
def update_manifest_shas_gensim_eks(ctx: Context) -> None:
    """
    Update pinned episode SHAs in the eval manifest to match the current gensim-episodes checkout.

    Only updates entries whose SHA has changed. Leaves unchanged entries untouched.
    Review and commit the result.

    Example:
        inv aws.eks.gensim.update-manifest-shas
    """
    if not _EVAL_MANIFEST_PATH.exists():
        raise Exit(f"Eval manifest not found: {_EVAL_MANIFEST_PATH}")

    try:
        entries = json.loads(_EVAL_MANIFEST_PATH.read_text())
    except json.JSONDecodeError as e:
        raise Exit(f"Failed to parse manifest: {e}") from e
    if not isinstance(entries, list):
        raise Exit(f"Manifest must be a JSON array, got {type(entries).__name__}.")

    gensim_repo_path = _get_gensim_repo_path()

    # ── Check gensim-episodes checkout state ──────────────────────────────
    branch_buf = StringIO()
    ctx.run(
        f"git -C {gensim_repo_path} rev-parse --abbrev-ref HEAD",
        out_stream=branch_buf,
        hide="out",
    )
    current_branch = branch_buf.getvalue().strip()

    ctx.run(f"git -C {gensim_repo_path} fetch origin main", hide=True, warn=True)
    behind_buf = StringIO()
    ctx.run(
        f"git -C {gensim_repo_path} rev-list --count HEAD..origin/main",
        out_stream=behind_buf,
        hide="out",
        warn=True,
    )
    behind_count = int(behind_buf.getvalue().strip() or "0")

    warnings = []
    if current_branch != "main":
        warnings.append(f"on '{current_branch}', not main")
    if behind_count > 0:
        warnings.append(f"behind origin/main by {behind_count} commit(s)")

    if warnings:
        tool.warn(f"Warning: gensim-episodes is {' and '.join(warnings)}.")
        tool.warn("SHAs pinned from this state may not match what others have locally.")
        answer = input("Continue anyway? [y/N]: ").strip().lower()
        if answer != "y":
            raise Exit("Aborted.")

    updated = 0
    for entry in entries:
        ep_name = entry.get("episode", "").strip()
        if not ep_name:
            continue
        ep_dir = _find_episode_dir(gensim_repo_path, ep_name)
        rel_path = ep_dir.relative_to(gensim_repo_path)
        sha_buf = StringIO()
        result = ctx.run(
            f"git -C {gensim_repo_path} rev-parse HEAD:{rel_path}",
            out_stream=sha_buf,
            hide="out",
            warn=True,
        )
        if not result.ok:
            tool.warn(f"Could not resolve SHA for '{ep_name}'. Skipping.")
            continue
        actual_sha = sha_buf.getvalue().strip()
        if actual_sha != entry["sha"]:
            tool.info(f"Updated {ep_name}: {entry['sha'][:12]} -> {actual_sha[:12]}")
            entry["sha"] = actual_sha
            updated += 1

    if updated == 0:
        tool.info("All SHAs are already up to date.")
        return

    _EVAL_MANIFEST_PATH.write_text(json.dumps(entries, indent=2) + "\n")
    tool.info(f"\nUpdated {updated} SHA(s) in {_EVAL_MANIFEST_PATH}. Review and commit the changes.")


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
    checked = [str(repo_path / ep_name)] + [str(repo_path / s / ep_name) for s in _EPISODE_SUBDIRS]
    raise Exit(f"Episode '{ep_name}' not found. Checked: {', '.join(checked)}")


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
