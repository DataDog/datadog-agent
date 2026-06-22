# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Shared dataclasses for demo scenario and option definitions.

Each team's scenario.py declares:
  - PULUMI_SCENARIO  — the registered scenario name (matches RegisterScenario in scenario.go)
  - CREATE_OPTIONS   — list of DemoOption describing dda lab demo <team> create flags
  - SCENARIOS        — dict of Scenario, each with named SSH actions (e.g. retrigger, reset)

The scenario_loader discovers these files automatically; no per-team
command files are needed.
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class DemoOption:
    """
    Declares one CLI option for 'dda lab demo <team> create' and its
    corresponding Pulumi config key.

    Example::

        DemoOption(
            name="site",
            pulumi_keys=["ddagent:site"],
            default="datad0g.com",
            help="Datadog site to report to",
        )

    Produces: --site (default datad0g.com) → -c ddagent:site=<value>
    """

    name: str
    """Python kwarg name.  Becomes --name (underscores → dashes) on the CLI."""

    pulumi_keys: list[str]
    """Pulumi config keys passed as -c <key>=<value> to pulumi up. All keys receive the same value."""

    help: str
    """Help text shown in --help output."""

    default: str | None = None
    """Default value; None means the option is optional with no default shown."""

    required: bool = False
    """If True the option is mandatory (no default)."""

    show_default: bool = True
    """Whether to show the default value in --help."""


@dataclass
class ScenarioAction:
    """SSH commands that implement one direction of a demo scenario."""

    commands: list[str]
    message: str


@dataclass
class Scenario:
    """A demo scenario with named SSH actions (e.g. retrigger, reset) for a live host."""

    issue: str
    description: str
    actions: dict[str, ScenarioAction]
    """Named SSH actions for this scenario, e.g. {"retrigger": ..., "remediate": ...}."""
    create_defaults: dict[str, str] = field(default_factory=dict)
    """Pulumi config key/value pairs automatically injected when
    'dda lab demo ... create --for-scenario <name>' is used.
    Keys must be fully-qualified Pulumi config keys (e.g. 'demolab:scenario')."""


@dataclass
class DemoEnv:
    """
    One provisioner type for a team's demo lab (e.g. "aws", "kind").

    Each entry in a team's DEMO_ENVS dict becomes a subcommand group:

        dda lab demo <team> <env-name> create   [--api-key] [--id] [<create_options>...]
        dda lab demo <team> <env-name> run --scenario <name> --action <action>
        dda lab demo <team> <env-name> <scenario> reset   [--id]

    pulumi_scenario must match the name passed to registry.RegisterScenario
    in the team's scenario.go.
    """

    pulumi_scenario: str
    """Registered scenario name, e.g. "aws/agent-health-demo"."""

    description: str
    """Short description shown in --help."""

    create_options: list[DemoOption] = field(default_factory=list)
    """CLI options forwarded as -c <pulumi_key>=<value> to pulumi up."""

    scenarios: dict[str, Scenario] = field(default_factory=dict)
    """Named scenarios available once the env is running, each with SSH actions."""

    host_required: bool = True
    """Set to False for scenarios that do not provision a remote host (e.g. pure
    ECS or Kind environments). When False, the 'hostIP absent' warning is
    suppressed after create."""
