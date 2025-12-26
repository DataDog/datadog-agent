# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="AWS infrastructure commands (VMs, EKS, ECS, Kind, Docker)",
)
def cmd() -> None:
    """
    AWS infrastructure commands for creating test environments.

    Available resources:
    - vm: Virtual machines (EC2 instances)
    - eks: Elastic Kubernetes Service clusters
    - ecs: Elastic Container Service clusters
    - kind: Kubernetes in Docker on EC2
    - docker: Docker environments on EC2
    - installer: Installer lab environments
    """
    pass

