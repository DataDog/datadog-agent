import json
import os
import subprocess
import time
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
        "episode": "Episode directory name (e.g., 002_AWS_S3_Service_Disruption)",
        "scenario": "Scenario name to execute (from episodes/*.yaml)",
        "stack_name": doc.stack_name,
        "agent_version": doc.container_agent_version,
        "config_path": "Path to config file",
        "interactive": "Enable interactive mode",
        "namespace": "Kubernetes namespace for deployment (default: default)",
    }
)
def deploy_gensim(
    ctx: Context,
    episode: str,
    scenario: str | None = None,
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
    Deploy a gensim episode to an EC2+Kind cluster, execute scenarios, and collect results.

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

    # Find episode
    gensim_path = get_gensim_repo_path()
    episode_dir = gensim_path / episode

    if not episode_dir.exists():
        raise Exit(f"Episode directory not found: {episode_dir}")

    chart_dir = episode_dir / "chart"
    if not chart_dir.exists():
        raise Exit(f"Chart directory not found: {chart_dir}")

    datadog_values_path = gensim_path / "datadog-values.yaml"

    # Prepare extra flags for gensim scenario
    extra_flags = {
        "ddinfra:osDescriptor": "amazonlinuxecs::x86_64",
        "ddinfra:aws/defaultInstanceType": "t3.xlarge",
        "gensim:episodeName": episode,
        "gensim:chartPath": str(chart_dir),
        "gensim:namespace": namespace,
    }

    if datadog_values_path.exists():
        extra_flags["gensim:datadogValuesPath"] = str(datadog_values_path)

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

    # Get kubeconfig from the Kind cluster output
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    kubeconfig_output = outputs["dd-Cluster-gensim"]["kubeConfig"]

    import yaml

    kubeconfig_content = yaml.dump(yaml.safe_load(kubeconfig_output))
    kubeconfig_path = Path(f"{full_stack_name}-config.yaml")

    with open(kubeconfig_path, "w") as f:
        f.write(kubeconfig_content)
    os.chmod(kubeconfig_path, 0o600)

    tool.info(f"Kubeconfig saved to: {kubeconfig_path}")

    # Wait for pods to be ready
    tool.info("Waiting for pods to be ready...")
    time.sleep(30)

    # Execute episode scenarios
    play_script = episode_dir / "play-episode.sh"

    if not play_script.exists():
        tool.warn(f"play-episode.sh not found in {episode_dir}")
    else:
        tool.info("Executing episode scenario(s)...")

        # Set environment variables for play-episode.sh
        env = os.environ.copy()
        env.update(
            {
                "DD_API_KEY": cfg.get_agent().apiKey or "",
                "DD_APP_KEY": cfg.get_agent().appKey or "",
                "DD_ENV": f"gensim-{episode}",
                "DD_SITE": cfg.get_site() if hasattr(cfg, 'get_site') else "datadoghq.com",
                "KUBE_NAMESPACE": namespace or "",
                "KUBECONFIG": str(kubeconfig_path),
            }
        )

        if scenario:
            # Run specific scenario
            tool.info(f"Running scenario: {scenario}")
            cmd = [str(play_script), "run-episode", scenario]
            subprocess.run(cmd, cwd=episode_dir, env=env, check=True)
        else:
            # List and prompt for scenario
            tool.info("Listing available scenarios...")
            result = subprocess.run(
                [str(play_script), "list-episodes"],
                cwd=episode_dir,
                capture_output=True,
                text=True,
                check=True,
            )
            scenarios_json = json.loads(result.stdout)

            if not scenarios_json:
                tool.warn("No scenarios found")
            else:
                tool.info(f"Available scenarios: {[s['name'] for s in scenarios_json]}")

                if interactive:
                    print("\nRun scenarios manually with:")
                    print(f"  export KUBECONFIG={kubeconfig_path}")
                    print(f"  cd {episode_dir}")
                    print("  ./play-episode.sh run-episode <scenario-name>")
                else:
                    # Run all scenarios in non-interactive mode
                    for scenario_info in scenarios_json:
                        scenario_name_run = scenario_info["name"]
                        tool.info(f"Running scenario: {scenario_name_run}")
                        cmd = [str(play_script), "run-episode", scenario_name_run]
                        subprocess.run(cmd, cwd=episode_dir, env=env, check=True)

    # Instructions for collecting parquet files
    tool.info("\n" + "=" * 80)
    tool.info("Episode deployment complete!")
    tool.info("=" * 80)
    print(f"\nKubeconfig: {kubeconfig_path}")
    print(f"Namespace: {namespace}")
    print("\nTo collect parquet files from Datadog Agent pods:")
    print(f"  kubectl --kubeconfig={kubeconfig_path} -n {namespace} get pods -l app=datadog-agent")
    print(f"  kubectl --kubeconfig={kubeconfig_path} -n {namespace} exec <pod-name> -- ls /tmp/observer-metrics/")
    print(
        f"  kubectl --kubeconfig={kubeconfig_path} -n {namespace} cp <pod-name>:/tmp/observer-metrics/ ./parquet_files/"
    )

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


@task(
    help={
        "stack_name": doc.stack_name,
        "namespace": "Kubernetes namespace where Datadog Agent is deployed",
        "output_dir": "Directory to save parquet files (default: ./parquet_files)",
    }
)
def collect_parquet_files(
    ctx: Context,
    stack_name: str,
    namespace: str | None = "default",
    output_dir: str | None = "./parquet_files",
):
    """
    Collect parquet files from Datadog Agent pods in a running gensim deployment.

    Example:
        inv e2e-framework.aws.collect-parquet-files --stack-name=gensim-002-aws-s3
    """

    # Get the full stack name
    full_stack_name = tool.get_stack_name(stack_name, scenario_name)

    # Get kubeconfig from the Kind cluster output
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    kubeconfig_output = outputs["dd-Cluster-gensim"]["kubeConfig"]

    import yaml

    kubeconfig_content = yaml.dump(yaml.safe_load(kubeconfig_output))
    kubeconfig_path = Path(f"{full_stack_name}-config.yaml")

    with open(kubeconfig_path, "w") as f:
        f.write(kubeconfig_content)
    os.chmod(kubeconfig_path, 0o600)

    # Create output directory
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    # Get Datadog Agent pods
    result = subprocess.run(
        [
            "kubectl",
            "--kubeconfig",
            str(kubeconfig_path),
            "get",
            "pods",
            "-n",
            namespace,
            "-l",
            "app=datadog-agent",
            "-o",
            "jsonpath={.items[*].metadata.name}",
        ],
        capture_output=True,
        text=True,
        check=True,
    )

    pod_names = result.stdout.strip().split()

    if not pod_names:
        tool.warn("No Datadog Agent pods found")
        return

    tool.info(f"Found {len(pod_names)} Datadog Agent pod(s)")

    # Collect parquet files from each pod
    for pod_name in pod_names:
        tool.info(f"Collecting from pod: {pod_name}")

        pod_output_dir = output_path / pod_name
        pod_output_dir.mkdir(exist_ok=True)

        # Copy parquet files
        subprocess.run(
            [
                "kubectl",
                "--kubeconfig",
                str(kubeconfig_path),
                "cp",
                f"{namespace}/{pod_name}:/tmp/observer-metrics/",
                str(pod_output_dir),
            ],
            check=False,  # Don't fail if directory doesn't exist
        )

        # Count parquet files
        parquet_files = list(pod_output_dir.rglob("*.parquet"))
        if parquet_files:
            tool.info(f"  Collected {len(parquet_files)} parquet file(s)")
        else:
            tool.warn(f"  No parquet files found in pod {pod_name}")

    tool.notify(ctx, f"Parquet files collected to: {output_path}")
