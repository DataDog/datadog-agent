# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Kind cluster management utilities."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from dda.cli.application import Application


def cluster_exists(app: Application, name: str) -> bool:
    """Check if a kind cluster with the given name exists."""
    return name in get_clusters(app)


def get_clusters(app: Application) -> list[str]:
    """Get list of existing kind clusters."""
    result = app.subprocess.capture(["kind", "get", "clusters"], check=False)
    clusters = result.strip().split("\n")
    return [c for c in clusters if c]


def create_cluster(app: Application, name: str, config_path: str, node_image: str) -> None:
    """Create a kind cluster."""
    app.subprocess.run(
        ["kind", "create", "cluster", "--name", name, "--config", config_path, "--image", node_image, "--wait", "60s"],
        check=True,
    )


def delete_cluster(app: Application, name: str) -> None:
    """Delete a kind cluster."""
    app.subprocess.run(["kind", "delete", "cluster", "--name", name], check=True)


def show_cluster_info(app: Application, name: str) -> None:
    """Show cluster info."""
    app.subprocess.run(["kubectl", "cluster-info", "--context", f"kind-{name}"], check=False)


def load_image(app: Application, cluster_name: str, image: str) -> None:
    """Load a docker image into a kind cluster."""
    app.display_info(f"Loading image '{image}' into kind cluster '{cluster_name}'...")
    app.subprocess.run(["kind", "load", "docker-image", image, "--name", cluster_name], check=True)
