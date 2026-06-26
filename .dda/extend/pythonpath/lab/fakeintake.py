# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Fakeintake deployment and management utilities."""

from __future__ import annotations

import os
import subprocess
import tempfile
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from dda.cli.application import Application


FAKEINTAKE_IMAGE = "datadog/fakeintake:latest"
FAKEINTAKE_DEPLOYMENT_YAML = """
apiVersion: v1
kind: Namespace
metadata:
  name: fakeintake
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fakeintake
  namespace: fakeintake
spec:
  replicas: 1
  selector:
    matchLabels:
      app: fakeintake
  template:
    metadata:
      labels:
        app: fakeintake
    spec:
      containers:
      - name: fakeintake
        image: {image}
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 80
          name: http
---
apiVersion: v1
kind: Service
metadata:
  name: fakeintake
  namespace: fakeintake
spec:
  selector:
    app: fakeintake
  ports:
  - port: 80
    targetPort: 80
    protocol: TCP
    name: http
  type: ClusterIP
"""


def ensure_fakeintake_image(app: Application) -> str:
    """
    Ensure fakeintake image is available (pull from Docker Hub if needed).

    Returns:
        The image tag.
    """
    # Check if image exists locally by trying to inspect it
    # Use subprocess.run directly for silent check
    try:
        subprocess.run(
            ["docker", "image", "inspect", FAKEINTAKE_IMAGE],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        # Image exists
        return FAKEINTAKE_IMAGE
    except subprocess.CalledProcessError:
        # Image doesn't exist, pull it
        app.display_info("Pulling fakeintake image from Docker Hub...")
        app.subprocess.run(["docker", "pull", FAKEINTAKE_IMAGE], check=True)
        app.display_success(f"Fakeintake image pulled: {FAKEINTAKE_IMAGE}")
        return FAKEINTAKE_IMAGE


def deploy_fakeintake(app: Application, cluster_name: str) -> str:
    """
    Deploy fakeintake to the kind cluster.

    Args:
        app: Application instance.
        cluster_name: Name of the kind cluster.

    Returns:
        The fakeintake service URL (cluster-internal).
    """
    app.display_info("Deploying fakeintake to kind cluster...")

    # Note: We don't need to load the image into kind anymore.
    # The cluster will pull directly from Docker Hub since imagePullPolicy is IfNotPresent.

    # Apply the deployment YAML
    deployment_yaml = FAKEINTAKE_DEPLOYMENT_YAML.format(image=FAKEINTAKE_IMAGE)
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
        f.write(deployment_yaml)
        yaml_path = f.name

    try:
        app.subprocess.run(
            ["kubectl", "apply", "-f", yaml_path, "--context", f"kind-{cluster_name}"],
            check=True,
        )
    finally:
        os.unlink(yaml_path)

    # Wait for the deployment to be ready
    app.display_info("Waiting for fakeintake to be ready...")
    app.subprocess.run(
        [
            "kubectl",
            "wait",
            "--for=condition=available",
            "--timeout=120s",
            "deployment/fakeintake",
            "-n",
            "fakeintake",
            "--context",
            f"kind-{cluster_name}",
        ],
        check=True,
    )

    app.display_success("Fakeintake deployed successfully!")

    # Return the service URL (accessible from within the cluster)
    return "http://fakeintake.fakeintake.svc.cluster.local"
