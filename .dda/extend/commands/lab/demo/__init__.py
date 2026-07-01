# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Demo environments and scenarios for agent teams.

Environments can be any Pulumi-backed infrastructure — AWS EC2, Kind
clusters, ECS, Kubernetes, etc. — declared as DEMO_ENVS entries in each
team's scenario.py.

Subcommands are auto-discovered from test/new-e2e/tests/*/scenario.py.
Each team with a scenario.py appears as:

    dda lab demo <team> <env> create  [--api-key] [--id] [<env-options>...]
    dda lab demo <team> <env> run     --scenario <name> --action <action> [--id] [--list]

To delete a demo environment use: dda lab delete --id <id>

To add a new team or env type, use the /create-demo-scenario skill or
create the files manually:
  - test/new-e2e/tests/<team>/scenario.go  (Go Pulumi provisioner)
  - test/new-e2e/tests/<team>/scenario.py  (DEMO_ENVS dict)
Then run: dda inv e2e-framework.generate-scenario-imports
"""

from __future__ import annotations

from lab.demo.scenario_loader import create_demo_group

cmd = create_demo_group()
