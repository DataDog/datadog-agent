# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from dda.cli.application import Application

DEFAULT_LOCAL_IMAGE_TAG = "datadog/agent-dev:local"


def build_local_image(
    app: Application,
    tag: str = DEFAULT_LOCAL_IMAGE_TAG,
    devenv: str = "",
    build_command: str = "",
) -> str:
    """
    Build a local agent image using hacky-dev-image-build.

    Runs the build inside a devcontainer to ensure proper build environment.

    Args:
        tag: Docker image tag for the built image.
        process_agent: Include process-agent in the build.
        trace_agent: Include trace-agent in the build.
        system_probe: Include system-probe in the build.
        security_agent: Include security-agent in the build.
        devenv: Developer environment ID to use (default: "default").

    Returns:
        The image tag.
    """
    app.display_info(f"Building local agent image with tag '{tag}'...")

    cmd = [
        "dda",
        "inv",
        "agent.hacky-dev-image-build",
        "--target-image",
        tag,
    ]

    if build_command:
        cmd = build_command.split(" ")

    # Run in devcontainer for proper build environment
    devenv_cmd = ["dda", "env", "dev", "run"]
    if devenv:
        devenv_cmd.extend(["--id", devenv])
    devenv_cmd.append("--")

    full_cmd = devenv_cmd + cmd
    app.subprocess.run(full_cmd, check=True)

    return tag


def install_with_helm(
    app: Application,
    *,
    cluster_name: str,
    api_key: str,
    app_key: str = "",
    agent_image: str = "",
    cluster_agent_image: str = "",
    helm_values: str | None = None,
    image_pull_policy: str = "IfNotPresent",
    fakeintake_url: str | None = None,
) -> None:
    """Install Datadog Agent using Helm."""
    app.display_info("Installing Datadog Agent via Helm...")

    # Add Datadog Helm repo
    app.subprocess.run(["helm", "repo", "add", "datadog", "https://helm.datadoghq.com"], check=False)
    app.subprocess.run(["helm", "repo", "update"], check=True)

    # Build helm install command
    helm_cmd = [
        "helm",
        "upgrade",
        "--install",
        "datadog",
        "datadog/datadog",
        "--namespace",
        "datadog",
        "--create-namespace",
        "--set",
        f"datadog.apiKey={api_key}",
        "--set",
        f"datadog.clusterName={cluster_name}",
        "--set",
        "datadog.kubelet.tlsVerify=false",
        "--set",
        "agents.useHostNetwork=true",
        "--set",
        "clusterAgent.enabled=true",
        "--set",
        "clusterAgent.metricsProvider.enabled=true",
    ]

    # Configure fakeintake endpoints if enabled
    if fakeintake_url:
        app.display_info(f"Configuring agent to use fakeintake at {fakeintake_url}")
        # Set the main intake URL to fakeintake
        # This will redirect metrics, traces, and other data to fakeintake
        helm_cmd.extend(
            [
                "--set",
                f"datadog.dd_url={fakeintake_url}",
                "--set",
                "datadog.logs.enabled=true",
                "--set",
                "datadog.logs.containerCollectAll=true",
            ]
        )

    if app_key:
        helm_cmd.extend(["--set", f"datadog.appKey={app_key}"])

    if agent_image:
        repo, image_tag = _parse_image(agent_image)
        helm_cmd.extend(
            [
                "--set",
                f"agents.image.repository={repo}",
                "--set",
                f"agents.image.tag={image_tag}",
                "--set",
                "agents.image.doNotCheckTag=true",
                "--set",
                f"agents.image.pullPolicy={image_pull_policy}",
            ]
        )

    if cluster_agent_image:
        repo, image_tag = _parse_image(cluster_agent_image)
        helm_cmd.extend(
            [
                "--set",
                f"clusterAgent.image.repository={repo}",
                "--set",
                f"clusterAgent.image.tag={image_tag}",
                "--set",
                f"clusterAgent.image.pullPolicy={image_pull_policy}",
            ]
        )

    if helm_values:
        helm_cmd.extend(["-f", helm_values])

    app.subprocess.run(helm_cmd, check=True)

    app.display_success("Datadog Agent installed successfully!")
    app.display_info("""
To check agent status:
    kubectl get pods -n datadog
    kubectl exec -it $(kubectl get pods -n datadog -l app=datadog -o jsonpath='{.items[0].metadata.name}') -n datadog -- agent status
""")


def _parse_image(image: str) -> tuple[str, str]:
    """Parse image string into (repository, tag)."""
    if ":" in image:
        return image.rsplit(":", 1)
    return image, "latest"
