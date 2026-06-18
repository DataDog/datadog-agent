# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

PROFILE_FILENAME = ".test_infra_config.yaml"


@dataclass
class AwsConfig:
    """AWS-specific configuration."""

    key_pair_name: str | None = None
    public_key_path: str | None = None
    private_key_path: str | None = None
    private_key_password: str | None = None
    account: str | None = None
    team_tag: str | None = None

    def get_account(self) -> str:
        if self.account is None:
            return "agent-sandbox"
        return self.account


@dataclass
class AzureConfig:
    """Azure-specific configuration."""

    public_key_path: str | None = None
    account: str = "agent-sandbox"


@dataclass
class GCPConfig:
    """GCP-specific configuration."""

    public_key_path: str | None = None
    pull_secret_path: str | None = None
    account: str = "agent-sandbox"


@dataclass
class LocalConfig:
    """Local environment configuration."""

    public_key_path: str | None = None


@dataclass
class AgentConfig:
    """Datadog Agent configuration."""

    api_key: str | None = None
    app_key: str | None = None
    verify_code_signature: bool = True


@dataclass
class PulumiConfig:
    """Pulumi-specific configuration."""

    passphrase: str | None = None
    log_level: int | None = None
    log_to_stderr: bool | None = None
    verbose_progress_streams: bool | None = None


@dataclass
class LabConfig:
    """Main configuration for lab environments."""

    aws: AwsConfig = field(default_factory=AwsConfig)
    azure: AzureConfig = field(default_factory=AzureConfig)
    gcp: GCPConfig = field(default_factory=GCPConfig)
    local: LocalConfig = field(default_factory=LocalConfig)
    agent: AgentConfig = field(default_factory=AgentConfig)
    pulumi: PulumiConfig = field(default_factory=PulumiConfig)
    stack_params: dict[str, dict[str, str]] = field(default_factory=dict)
    dev_mode: bool = False

    @classmethod
    def from_dict(cls, data: dict[str, Any] | None) -> LabConfig:
        """Create a LabConfig from a dictionary (parsed YAML)."""
        if data is None:
            return cls()

        config_params = data.get("configParams", {}) or {}
        stack_params = data.get("stackParams", {}) or {}

        # Parse AWS config
        aws_data = config_params.get("aws", {}) or {}
        aws = AwsConfig(
            key_pair_name=aws_data.get("keyPairName"),
            public_key_path=aws_data.get("publicKeyPath"),
            private_key_path=aws_data.get("privateKeyPath"),
            private_key_password=aws_data.get("privateKeyPassword"),
            account=aws_data.get("account"),
            team_tag=aws_data.get("teamTag"),
        )

        # Parse Azure config
        azure_data = config_params.get("azure", {}) or {}
        azure = AzureConfig(
            public_key_path=azure_data.get("publicKeyPath"),
            account=azure_data.get("account", "agent-sandbox"),
        )

        # Parse GCP config
        gcp_data = config_params.get("gcp", {}) or {}
        gcp = GCPConfig(
            public_key_path=gcp_data.get("publicKeyPath"),
            pull_secret_path=gcp_data.get("pullSecretPath"),
            account=gcp_data.get("account", "agent-sandbox"),
        )

        # Parse Local config
        local_data = config_params.get("local", {}) or {}
        local = LocalConfig(
            public_key_path=local_data.get("publicKeyPath"),
        )

        # Parse Agent config
        agent_data = config_params.get("agent", {}) or {}
        agent = AgentConfig(
            api_key=agent_data.get("apiKey"),
            app_key=agent_data.get("appKey"),
            verify_code_signature=agent_data.get("verifyCodeSignature", True),
        )

        # Parse Pulumi config
        pulumi_data = config_params.get("pulumi", {}) or {}
        pulumi = PulumiConfig(
            passphrase=pulumi_data.get("passphrase"),
            log_level=pulumi_data.get("logLevel"),
            log_to_stderr=pulumi_data.get("logToStdErr"),
            verbose_progress_streams=pulumi_data.get("verboseProgressStreams"),
        )

        return cls(
            aws=aws,
            azure=azure,
            gcp=gcp,
            local=local,
            agent=agent,
            pulumi=pulumi,
            stack_params=stack_params,
            dev_mode=config_params.get("devMode", False),
        )

    def get_api_key(self) -> str | None:
        """Get API key from config or environment variable."""
        # First try environment variable
        api_key = os.environ.get("E2E_API_KEY")
        if api_key:
            return api_key
        # Fall back to config
        return self.agent.api_key

    def get_app_key(self) -> str | None:
        """Get APP key from config or environment variable."""
        # First try environment variable
        app_key = os.environ.get("E2E_APP_KEY")
        if app_key:
            return app_key
        # Fall back to config
        return self.agent.app_key


def get_config_path(config_path: str | None = None) -> Path:
    """Get the full path to the config file."""
    if config_path:
        return Path(config_path).expanduser().absolute()
    return Path.home() / PROFILE_FILENAME


def _minimal_yaml_load(path: Path) -> dict | None:
    """Parse ~/.test_infra_config.yaml without the yaml module.

    Handles nested dicts and scalar values (strings, bools, null).
    Does not support YAML lists, anchors, or multi-line values — none
    of which appear in the test infra config format.
    """
    try:
        text = path.read_text()
    except FileNotFoundError:
        return None

    root: dict = {}
    path_stack: list[dict] = [root]
    indent_stack: list[int] = [-1]

    for line in text.splitlines():
        stripped = line.lstrip()
        if not stripped or stripped.startswith("#"):
            continue
        indent = len(line) - len(stripped)

        # Pop back to the enclosing level
        while indent_stack[-1] >= indent:
            indent_stack.pop()
            path_stack.pop()

        if ":" not in stripped:
            continue
        key_part, _, rest = stripped.partition(":")
        key = key_part.strip()
        value = rest.strip()

        # Strip YAML quotes
        if len(value) >= 2 and value[0] in ('"', "'") and value[-1] == value[0]:
            value = value[1:-1]

        if value in ("", "null"):
            if value == "null":
                path_stack[-1][key] = None
            else:
                child: dict = {}
                path_stack[-1][key] = child
                path_stack.append(child)
                indent_stack.append(indent)
        elif value.lower() == "true":
            path_stack[-1][key] = True
        elif value.lower() == "false":
            path_stack[-1][key] = False
        else:
            path_stack[-1][key] = value

    return root


def load_config(config_path: str | None = None) -> LabConfig:
    """Load configuration from file."""
    path = get_config_path(config_path)
    try:
        try:
            import yaml  # noqa: PLC0415 — deferred; absent in dda's bundled Python env

            with open(path) as f:
                data = yaml.safe_load(f)
        except ImportError:
            data = _minimal_yaml_load(path)
    except FileNotFoundError:
        return LabConfig()
    except Exception as e:
        raise ValueError(f"Error parsing config file {path}: {e}") from e
    return LabConfig.from_dict(data)
