# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Agent health demo environment and scenario definitions.

Discovered automatically by lab.demo.scenario_loader — no changes to
.dda/extend/ are needed when adding entries here.

  DEMO_ENVS  maps env-type names to DemoEnv instances, each declaring:
               - pulumi_scenario  matches RegisterScenario in scenario.go
               - create_options   CLI flags for 'dda lab demo agent-health <env> create'
               - scenarios        retrigger/remediate SSH actions for that env type

Issue coverage (vm env type):
  docker_file_tailing_disabled   — Docker socket permission denied
  check_execution_failure        — Custom check that always raises an exception
  invalid-config                 — Unrecognised key injected into datadog.yaml
"""

from __future__ import annotations

from lab.demo.scenarios import DemoEnv, DemoOption, Scenario, ScenarioAction

DEMO_ENVS: dict[str, DemoEnv] = {
    "vm": DemoEnv(
        pulumi_scenario="aws/agent-health-demo",
        description="AWS EC2 host with Docker and busybox containers",
        create_options=[
            DemoOption(
                name="infra_env",
                pulumi_keys=["ddinfra:env"],
                default="aws/agent-sandbox",
                help="Pulumi infra environment name (e.g. aws/agent-sandbox)",
            ),
            DemoOption(
                name="site",
                pulumi_keys=["ddagent:site"],
                default="datad0g.com",
                help="Datadog site to report to (e.g. datad0g.com, datadoghq.com)",
            ),
            DemoOption(
                name="agent_version",
                pulumi_keys=["ddagent:version"],
                default=None,
                help="Explicit agent version to install, e.g. 7.57.0 (overrides --pipeline-id)",
            ),
            DemoOption(
                name="pipeline_id",
                pulumi_keys=["ddagent:pipeline_id"],
                default=None,
                help="CI pipeline ID whose build artifacts to install",
            ),
            DemoOption(
                name="tags",
                pulumi_keys=["ddinfra:extraResourcesTags", "ddagent:tags"],
                default=None,
                help="Comma-separated tags applied to AWS resources and the Datadog agent, e.g. 'env:demo,team:agent-health'",
            ),
            DemoOption(
                name="fakeintake",
                pulumi_keys=["ddagent:fakeintake"],
                default="false",
                help="Enable fakeintake instead of reporting to real Datadog (for E2E tests)",
            ),
        ],
        scenarios={
            "docker-permissions": Scenario(
                issue="docker_file_tailing_disabled",
                description="Lock down Docker socket to owner-only to trigger log tailing issue",
                actions={
                    "retrigger": ScenarioAction(
                        commands=[
                            "sudo chmod 660 /var/run/docker.sock",
                            "sudo systemctl restart datadog-agent",
                        ],
                        message="Docker socket locked to root-only — agent can no longer tail container logs.",
                    ),
                    "remediate": ScenarioAction(
                        commands=[
                            "sudo chmod 666 /var/run/docker.sock",
                            "sudo systemctl stop datadog-agent",
                            "sudo systemctl restart datadog-agent",
                        ],
                        message="Docker socket permissions restored — issue cleared.",
                    ),
                },
                create_defaults={},
            ),
            "check-failure": Scenario(
                issue="check_execution_failure",
                description="Deploy a custom check that always raises an exception",
                actions={
                    "retrigger": ScenarioAction(
                        commands=[
                            "sudo mkdir -p /etc/datadog-agent/checks.d",
                            "printf 'from datadog_checks.base import AgentCheck\\nclass BrokenCheck(AgentCheck):\\n    def check(self, instance):\\n        raise RuntimeError(\"Intentional error for demo purposes\")\\n' | sudo tee /etc/datadog-agent/checks.d/broken_check.py > /dev/null",
                            "sudo mkdir -p /etc/datadog-agent/conf.d/broken_check.d",
                            "printf 'init_config:\\ninstances:\\n  - {}\\n' | sudo tee /etc/datadog-agent/conf.d/broken_check.d/conf.yaml > /dev/null",
                            "sudo systemctl restart datadog-agent",
                        ],
                        message="Broken check deployed — check_execution_failure issue will appear after the first collection cycle.",
                    ),
                    "remediate": ScenarioAction(
                        commands=[
                            "sudo rm -f /etc/datadog-agent/checks.d/broken_check.py",
                            "sudo rm -rf /etc/datadog-agent/checks.d/__pycache__",
                            "sudo rm -rf /etc/datadog-agent/conf.d/broken_check.d",
                            "sudo systemctl restart datadog-agent",
                        ],
                        message="Broken check removed — issue cleared.",
                    ),
                },
                create_defaults={},
            ),
            "invalid-config": Scenario(
                issue="invalid-config",
                description="Inject a type-mismatched value into datadog.yaml to trigger schema validation failure",
                actions={
                    "retrigger": ScenarioAction(
                        commands=[
                            "echo 'check_runners: \"not_a_number_for_demo\"' | sudo tee -a /etc/datadog-agent/datadog.yaml > /dev/null",
                            "sudo systemctl restart datadog-agent",
                        ],
                        message="Invalid config value injected — invalid-config issue will appear after the agent validates its configuration.",
                    ),
                    "remediate": ScenarioAction(
                        commands=[
                            "sudo sed -i '/^check_runners:/d' /etc/datadog-agent/datadog.yaml",
                            "sudo systemctl restart datadog-agent",
                        ],
                        message="Invalid config value removed — issue cleared.",
                    ),
                },
                create_defaults={},
            ),
        },
    ),
}
