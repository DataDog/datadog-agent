import json
import os
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/gensim"


def get_gensim_repo_path() -> Path:
    """Get the path to the gensim-episodes repository"""
    # Try environment variable first
    env_path = os.getenv("GENSIM_REPO_PATH")
    if env_path:
        path = Path(env_path)
        if path.exists():
            return path

    # Try to find it relative to datadog-agent repo
    # Assume both repos are in the same parent directory
    current_dir = Path(__file__).parent
    repo_root = current_dir.parent.parent.parent  # Go up to datadog-agent root
    parent_dir = repo_root.parent

    possible_paths = [
        parent_dir / "gensim-episodes" / "postmortems",
        Path.home() / "go" / "src" / "github.com" / "DataDog" / "gensim-episodes" / "postmortems",
    ]

    for path in possible_paths:
        if path.exists():
            return path

    raise Exit("Could not find gensim-episodes repository. Set GENSIM_REPO_PATH environment variable.")


def list_episodes() -> list[dict]:
    """List all available gensim episodes"""
    gensim_path = get_gensim_repo_path()
    episodes = []

    for episode_dir in sorted(gensim_path.iterdir()):
        if not episode_dir.is_dir():
            continue

        # Check if it's an episode directory
        chart_dir = episode_dir / "chart"
        play_script = episode_dir / "play-episode.sh"

        if chart_dir.exists() and play_script.exists():
            # Get scenarios for this episode
            scenarios = []
            episodes_subdir = episode_dir / "episodes"
            if episodes_subdir.exists():
                for scenario_file in episodes_subdir.glob("*.yaml"):
                    scenarios.append(scenario_file.stem)

            episodes.append(
                {
                    "name": episode_dir.name,
                    "path": str(episode_dir),
                    "chart_path": str(chart_dir),
                    "scenarios": scenarios,
                }
            )

    return episodes


@task
def list_gensim_episodes(ctx: Context):
    """
    List all available gensim episodes.
    """
    episodes = list_episodes()
    print(json.dumps(episodes, indent=2))


@task(
    help={
        "episode": "Episode directory name (e.g., 002_AWS_S3_Service_Disruption);",
        "scenario": "Scenario name to run autonomously on the VM (from episodes/*.yaml)",
        "s3_bucket": "S3 bucket to upload results archive (default: qbranch-gensim-recordings)",
        "instance_type": "EC2 instance type (default: t3.xlarge)",
        "stack_name": doc.stack_name,
        "agent_version": doc.container_agent_version,
        "config_path": "Path to config file",
        "interactive": "Enable interactive mode",
        "namespace": "Kubernetes namespace for deployment (default: default)",
    }
)
def deploy_gensim(
    ctx: Context,
    episode: str | None = None,
    scenario: str | None = None,
    s3_bucket: str | None = "qbranch-gensim-recordings",
    instance_type: str | None = "t3.xlarge",
    debug: bool | None = False,
    stack_name: str | None = None,
    agent_version: str | None = None,
    config_path: str | None = None,
    interactive: bool | None = True,
    full_image_path: str | None = None,
    cluster_agent_full_image_path: str | None = None,
    namespace: str | None = "default",
    keep_after_scenario: bool | None = False,
) -> None:
    """
    Deploy a gensim episode to an EC2+Kind cluster and run it autonomously on the VM.

    Example:
        inv e2e-framework.aws.deploy-gensim --episode=002_AWS_S3_Service_Disruption --scenario=capacity-removal-outage
    """

    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config
    from tasks.e2e_framework.config import get_full_profile_path

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {get_full_profile_path(config_path)}") from e

    # Find episode — if not specified, pick the first one available
    gensim_path = get_gensim_repo_path()

    episode_dir = gensim_path / episode

    if not episode_dir.exists():
        raise Exit(f"Episode directory not found: {episode_dir}")

    chart_dir = episode_dir / "chart"
    if not chart_dir.exists():
        raise Exit(f"Chart directory not found: {chart_dir}")

    # Auto-pick the first available scenario if none was specified
    if scenario is None:
        available = sorted((episode_dir / "episodes").glob("*.yaml"))
        if not available:
            raise Exit(f"No scenario files found in {episode_dir / 'episodes'}")
        scenario = available[0].stem
        tool.info(f"No scenario specified, using: {scenario}")

    datadog_values_path = gensim_path / "datadog-values.yaml"

    # Default stack name to the episode name so each episode gets its own stack
    if stack_name is None:
        stack_name = "gensim-" + episode.replace("_", "-").lower()

    # Prepare extra flags for gensim scenario
    extra_flags = {
        "ddinfra:osDescriptor": "amazonlinuxecs::x86_64",
        "ddinfra:aws/defaultInstanceType": instance_type,
        "gensim:episodeName": episode,
        "gensim:chartPath": str(chart_dir),
        "gensim:episodePath": str(episode_dir),
        "gensim:namespace": namespace,
    }

    if datadog_values_path.exists():
        extra_flags["gensim:datadogValuesPath"] = str(datadog_values_path)

    if scenario:
        extra_flags["gensim:scenario"] = scenario
    if s3_bucket:
        extra_flags["gensim:s3Bucket"] = s3_bucket

    # Deploy the infrastructure
    tool.notify(ctx, f"Deploying gensim episode: {episode}")

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path=config_path,
        key_pair_required=True,
        debug=debug,
        app_key_required=True,
        stack_name=stack_name,
        install_agent=True,  # Deploy custom Datadog Agent with observer
        install_workload=False,  # Episode chart includes workload
        agent_version=agent_version,
        extra_flags=extra_flags,
        full_image_path=full_image_path,
        cluster_agent_full_image_path=cluster_agent_full_image_path,
    )

    if interactive:
        tool.notify(ctx, f"Kind cluster created for episode: {episode}")

    # Get VM connection info from stack outputs
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    remote_host = tool.RemoteHost("aws-gensim", outputs)
    key_path = cfg.get_aws().privateKeyPath

    tool.info("\n" + "=" * 80)
    tool.info("Infrastructure is ready. Episode is running autonomously on the VM.")
    tool.info("=" * 80)
    ssh_key_flag = f"-i {key_path} " if key_path else ""
    print("\nTo monitor progress:")
    print(f"  ssh {ssh_key_flag}{remote_host.user}@{remote_host.address}")
    print("  tail -f /tmp/gensim-runner.log")

    if s3_bucket:
        print(f"\nResults will be uploaded to: s3://{s3_bucket}/gensim-results-{episode}-<YYYYMMDD>.zip")

    if not keep_after_scenario:
        print("\nTo destroy the cluster:")
        print(f"  inv e2e-framework.aws.destroy-gensim --stack-name={full_stack_name.split('/')[-1]}")
    else:
        tool.warn("Cluster will remain running. Remember to destroy it when done!")


@task(help={"stack_name": doc.stack_name})
def destroy_gensim(ctx: Context, stack_name: str | None = None, config_path: str | None = None):
    """
    Destroy a gensim EC2+Kind environment created with inv e2e-framework.aws.deploy-gensim.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name, config_path=config_path)
