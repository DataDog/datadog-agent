# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Base provider for Pulumi-managed infrastructure."""

from __future__ import annotations

import json
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING, Any

from lab.providers import BaseProvider, MissingPrerequisite, ProviderConfig, ProviderOptions

if TYPE_CHECKING:
    from dda.cli.application import Application


@dataclass
class PulumiOptions(ProviderOptions):
    """Options for Pulumi-based providers."""

    # Scenario configuration
    scenario: str  # e.g., "aws/eks", "gcp/gke"
    config_path: str | None = None  # Override ~/.test_infra_config.yaml

    # Deployment options
    install_agent: bool = True
    agent_version: str | None = None
    use_fakeintake: bool = False

    # Advanced options
    extra_flags: dict[str, Any] | None = None
    debug: bool = False

    # Cloud-provider specific (populated from config)
    api_key: str = ""
    app_key: str = ""

    @classmethod
    def from_config(cls, config: ProviderConfig) -> PulumiOptions:
        """Create PulumiOptions from ProviderConfig."""
        # Get scenario from config or infer from provider name
        scenario = config.options.get("scenario", "")

        return cls(
            name=config.name,
            scenario=scenario,
            config_path=config.options.get("config_path"),
            install_agent=config.options.get("install_agent", True),
            agent_version=config.options.get("agent_version"),
            use_fakeintake=config.options.get("use_fakeintake", False),
            extra_flags=config.options.get("extra_flags"),
            debug=config.options.get("debug", False),
            api_key=config.get_api_key() or "",
            app_key=config.get_app_key() or "",
        )


class PulumiProvider(BaseProvider):
    """Base provider for Pulumi-managed infrastructure.

    This provider wraps Pulumi scenarios from the e2e framework,
    providing a unified lab interface for cloud infrastructure.
    """

    name = "pulumi"
    category = "e2e"
    description = "Pulumi-managed infrastructure"
    options_class = PulumiOptions

    def __init__(self, scenario: str):
        """Initialize with a specific Pulumi scenario.

        Args:
            scenario: Pulumi scenario name (e.g., "aws/eks", "gcp/gke")
        """
        self.scenario = scenario

    def check_prerequisites(self, app: Application, opts: ProviderOptions) -> list[MissingPrerequisite]:
        """Check Pulumi and scenario prerequisites."""
        missing = []

        # Check Pulumi CLI (check PATH and common install locations)
        if not self._find_pulumi():
            missing.append(
                MissingPrerequisite(
                    name="Pulumi CLI",
                    remediation="Run: dda inv e2e.setup",
                    actions={"create", "delete"},
                )
            )

        # Check cloud provider CLI based on scenario
        provider = self.scenario.split("/")[0]

        if provider == "aws" and not shutil.which("aws"):
            missing.append(
                MissingPrerequisite(
                    name="AWS CLI",
                    remediation="Install AWS CLI: https://aws.amazon.com/cli/",
                    actions={"create"},
                )
            )
        elif provider == "gcp" and not shutil.which("gcloud"):
            missing.append(
                MissingPrerequisite(
                    name="Google Cloud SDK",
                    remediation="Install gcloud: https://cloud.google.com/sdk/docs/install",
                    actions={"create"},
                )
            )
        elif provider == "azure" and not shutil.which("az"):
            missing.append(
                MissingPrerequisite(
                    name="Azure CLI",
                    remediation="Install Azure CLI: https://docs.microsoft.com/en-us/cli/azure/install-azure-cli",
                    actions={"create"},
                )
            )

        # Check e2e configuration
        if not self._has_valid_config():
            missing.append(
                MissingPrerequisite(
                    name="E2E configuration",
                    remediation="Run: dda inv e2e.setup",
                    actions={"create"},
                )
            )

        return missing

    def create(self, app: Application, opts: ProviderOptions) -> dict[str, Any]:
        """Deploy infrastructure using Pulumi e2e framework."""
        from lab.pulumi_config import get_agent_keys

        options = opts if isinstance(opts, PulumiOptions) else PulumiOptions.from_config(ProviderConfig(opts.name, {}))

        # Get agent keys from config if not provided
        if not options.api_key or not options.app_key:
            api_key, app_key = get_agent_keys()
            options.api_key = options.api_key or api_key
            options.app_key = options.app_key or app_key

        app.display_info(f"Deploying Pulumi scenario: {self.scenario}")

        # Build deployment arguments for e2e framework
        deploy_args = self._build_deploy_args(options)

        # Execute Pulumi deployment via e2e framework
        stack_name = self._run_pulumi_deploy(app, deploy_args)

        # Get stack outputs
        outputs = self._get_stack_outputs(app, stack_name)

        app.display_success(f"Scenario '{self.scenario}' deployed as stack '{stack_name}'")

        # Show fakeintake instructions if enabled
        if options.use_fakeintake:
            app.display_info("\n📊 Fakeintake is ready! Query metrics with:")
            app.display_info(f"  dda lab query --id {options.name} --list")
            app.display_info(f"  dda lab query --id {options.name} --metric system.cpu.idle")

        return {
            "scenario": self.scenario,
            "stack_name": stack_name,
            "outputs": outputs,
            "fakeintake_enabled": options.use_fakeintake,
        }

    def destroy(self, app: Application, name: str) -> None:
        """Destroy Pulumi stack."""
        import os

        pulumi_cmd = self._get_pulumi_command()
        if not pulumi_cmd:
            app.abort("Pulumi CLI not found")

        app.display_info(f"Destroying Pulumi stack: {name}")

        # Set default passphrase for lab environments
        env = os.environ.copy()
        if "PULUMI_CONFIG_PASSPHRASE" not in env:
            env["PULUMI_CONFIG_PASSPHRASE"] = "datadog-lab"

        # Use pulumi destroy directly
        try:
            app.subprocess.run(
                [pulumi_cmd, "destroy", "--remove", "--yes", "--skip-preview", "-s", name],
                check=True,
                env=env,
            )
            app.display_success(f"Stack '{name}' destroyed")
        except subprocess.CalledProcessError:
            # Try with refresh if first attempt fails
            app.display_warning("Destroy failed, trying with --refresh...")
            app.subprocess.run(
                [pulumi_cmd, "destroy", "--remove", "--yes", "--skip-preview", "--refresh", "-s", name],
                check=True,
                env=env,
            )
            app.display_success(f"Stack '{name}' destroyed")

    def _build_deploy_args(self, options: PulumiOptions) -> dict[str, Any]:
        """Build arguments for e2e deploy function."""
        args = {
            "scenario_name": self.scenario,
            "stack_name": options.name,
            "install_agent": options.install_agent,
            "use_fakeintake": options.use_fakeintake,
            "debug": options.debug,
        }

        if options.config_path:
            args["config_path"] = options.config_path

        if options.agent_version:
            args["agent_version"] = options.agent_version

        if options.extra_flags:
            args["extra_flags"] = options.extra_flags

        return args

    def _run_pulumi_deploy(self, app: Application, deploy_args: dict[str, Any]) -> str:
        """Run Pulumi deployment using app.subprocess.

        Returns:
            Stack name
        """
        from lab import pulumi_config

        # Build flags dictionary
        flags = deploy_args.get("extra_flags", {}).copy()
        scenario_name = deploy_args["scenario_name"]
        flags["scenario"] = scenario_name

        # Agent deployment flags
        install_agent = deploy_args.get("install_agent", True)
        install_installer = deploy_args.get("install_installer", False)
        flags["ddagent:deploy"] = install_agent and not install_installer
        flags["ddupdater:deploy"] = install_installer

        # Workload flags
        install_workload = deploy_args.get("install_workload", pulumi_config.get_default_workload_install())
        flags["ddtestworkload:deploy"] = install_workload

        # Agent configuration
        flags["ddagent:version"] = deploy_args.get("agent_version")
        flags["ddagent:flavor"] = deploy_args.get("agent_flavor")
        flags["ddagent:fakeintake"] = deploy_args.get("use_fakeintake", False)
        flags["ddagent:fullImagePath"] = deploy_args.get("full_image_path")
        flags["ddagent:clusterAgentFullImagePath"] = deploy_args.get("cluster_agent_full_image_path")
        flags["ddagent:configPath"] = deploy_args.get("agent_config_path")
        flags["ddagent:extraEnvVars"] = deploy_args.get("agent_env")
        flags["ddagent:helmConfig"] = deploy_args.get("helm_config")
        flags["ddagent:localPackage"] = deploy_args.get("local_package")
        flags["ddagent:pipeline_id"] = deploy_args.get("pipeline_id", "")

        if install_agent:
            flags["ddagent:apiKey"] = pulumi_config.get_api_key()

        # Enable dual shipping for fakeintake
        if deploy_args.get("use_fakeintake"):
            flags["ddagent:dualshipping"] = True

        # Add stack params from config
        stack_params = pulumi_config.get_stack_params()
        for namespace in stack_params:
            for key, value in stack_params[namespace].items():
                flags[f"{namespace}:{key}"] = value

        # Add AWS-specific flags if not already set
        if scenario_name.startswith("aws/"):
            aws_config = pulumi_config.get_cloud_credentials("aws")
            if "ddinfra:aws/defaultKeyPairName" not in flags and "keyPairName" in aws_config:
                flags["ddinfra:aws/defaultKeyPairName"] = aws_config["keyPairName"]

        if deploy_args.get("app_key_required"):
            flags["ddagent:appKey"] = pulumi_config.get_app_key()

        # Build stack name
        stack_name = deploy_args.get("stack_name")
        stack_name = pulumi_config.get_stack_name(stack_name, scenario_name)
        stack_name = stack_name.replace(" ", "-").lower()

        # Add required ddinfra:env flag (environment name for resources)
        # Use aws/agent-sandbox as default for AWS scenarios
        if "ddinfra:env" not in flags:
            provider_name = scenario_name.split("/")[0] if "/" in scenario_name else "aws"
            flags["ddinfra:env"] = f"{provider_name}/agent-sandbox"

        # Build pulumi command
        pulumi_dir = pulumi_config.get_pulumi_directory()
        global_flags = [f"--cwd={pulumi_dir}"]

        # Add logging flags if debug enabled
        debug = deploy_args.get("debug", False)
        log_level = pulumi_config.get_pulumi_log_level()
        log_to_stderr = pulumi_config.get_pulumi_log_to_stderr()

        should_log = debug or log_level != 3 or log_to_stderr
        if should_log:
            if log_to_stderr or debug:
                global_flags.append("--logtostderr")
            global_flags.append(f"-v={log_level}")

        # Create stack if it doesn't exist
        self._ensure_stack_exists(app, stack_name, global_flags)

        # Build config flags for pulumi up
        config_flags = []
        for key, value in flags.items():
            if value is not None:
                config_flags.extend(["-c", f"{key}={value}"])

        # Get Pulumi command path
        pulumi_cmd = self._get_pulumi_command()
        if not pulumi_cmd:
            app.abort("Pulumi CLI not found")

        # Build final command
        cmd = [pulumi_cmd] + global_flags + ["up", "--yes", "-s", stack_name]
        if debug:
            cmd.append("--debug")
        cmd.extend(config_flags)

        # Run pulumi up
        app.display(f"Deploying Pulumi scenario: {scenario_name}")
        app.display(f"Stack name: {stack_name}")

        # Set default passphrase for lab environments if not already set
        import os

        env = os.environ.copy()
        if "PULUMI_CONFIG_PASSPHRASE" not in env:
            env["PULUMI_CONFIG_PASSPHRASE"] = "datadog-lab"

        result = app.subprocess.run(
            cmd,
            check=True,
            env=env,
        )

        if result.returncode != 0:
            app.abort(f"Pulumi deployment failed with exit code {result.returncode}")

        return stack_name

    def _ensure_stack_exists(self, app: Application, stack_name: str, global_flags: list[str]) -> None:
        """Create stack if it doesn't exist."""
        import os

        # Get Pulumi command path
        pulumi_cmd = self._get_pulumi_command()
        if not pulumi_cmd:
            app.abort("Pulumi CLI not found")

        # Check if stack exists
        cmd = [pulumi_cmd] + global_flags + ["stack", "ls", "--all"]
        try:
            output = app.subprocess.capture(cmd, check=False)
            if stack_name in output:
                return
        except subprocess.CalledProcessError:
            pass  # Stack doesn't exist, will create

        # Create stack with passphrase secrets provider
        # Use a default passphrase for lab environments if not set
        env = os.environ.copy()
        if "PULUMI_CONFIG_PASSPHRASE" not in env:
            env["PULUMI_CONFIG_PASSPHRASE"] = "datadog-lab"

        cmd = (
            [pulumi_cmd] + global_flags + ["stack", "init", "--no-select", "--secrets-provider=passphrase", stack_name]
        )
        app.subprocess.run(cmd, check=True, env=env)

    def _get_stack_outputs(self, app: Application, stack_name: str) -> dict[str, Any]:
        """Get outputs from Pulumi stack."""
        import os

        pulumi_cmd = self._get_pulumi_command()
        if not pulumi_cmd:
            return {}

        # Set default passphrase for lab environments
        env = os.environ.copy()
        if "PULUMI_CONFIG_PASSPHRASE" not in env:
            env["PULUMI_CONFIG_PASSPHRASE"] = "datadog-lab"

        try:
            result = app.subprocess.capture(
                [pulumi_cmd, "stack", "output", "--json", "-s", stack_name],
                check=True,
                env=env,
            )
            return json.loads(result)
        except (subprocess.CalledProcessError, json.JSONDecodeError):
            # Return empty dict if outputs not available yet
            return {}

    def _has_valid_config(self) -> bool:
        """Check if e2e configuration exists."""
        config_path = Path.home() / ".test_infra_config.yaml"
        return config_path.exists()

    def _find_pulumi(self) -> bool:
        """Find Pulumi CLI in PATH or common installation locations."""
        return self._get_pulumi_command() is not None

    def _get_pulumi_command(self) -> str | None:
        """Get the Pulumi command path."""
        # Check PATH first
        pulumi_path = shutil.which("pulumi")
        if pulumi_path:
            return pulumi_path

        # Check common installation locations
        common_paths = [
            Path.home() / ".pulumi" / "bin" / "pulumi",
            Path("/usr/local/bin/pulumi"),
            Path("/usr/bin/pulumi"),
        ]

        for path in common_paths:
            if path.exists() and path.is_file():
                return str(path)

        return None
