# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""Pulumi configuration management for lab environments.

This module provides access to the e2e framework configuration
stored in ~/.test_infra_config.yaml.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

import yaml


def load_pulumi_config() -> dict[str, Any]:
    """Load e2e + lab configuration from ~/.test_infra_config.yaml.

    Returns:
        Configuration dictionary, or empty dict if file doesn't exist
    """
    config_path = Path.home() / ".test_infra_config.yaml"

    if not config_path.exists():
        return {}

    try:
        with open(config_path) as f:
            config = yaml.safe_load(f)
            return config or {}
    except Exception:
        return {}


def get_lab_defaults() -> dict[str, Any]:
    """Get lab-specific defaults from configuration.

    Returns:
        Lab defaults dictionary
    """
    config = load_pulumi_config()
    return config.get("lab", {}).get("defaults", {})


def get_cloud_credentials(provider: str) -> dict[str, Any]:
    """Get credentials for a cloud provider.

    Args:
        provider: Cloud provider name ("aws", "gcp", "azure")

    Returns:
        Provider-specific configuration dictionary
    """
    config = load_pulumi_config()
    return config.get("configParams", {}).get(provider, {})


def get_agent_keys() -> tuple[str, str]:
    """Get Datadog agent API and App keys.

    Returns:
        Tuple of (api_key, app_key). Empty strings if not configured.
    """
    config = load_pulumi_config()
    agent_config = config.get("configParams", {}).get("agent", {})
    api_key = agent_config.get("apiKey", "")
    app_key = agent_config.get("appKey", "")
    return api_key, app_key


def has_valid_config() -> bool:
    """Check if e2e configuration file exists and is valid.

    Returns:
        True if configuration exists and can be loaded
    """
    config = load_pulumi_config()
    return bool(config and "configParams" in config)


def get_stack_params() -> dict[str, dict[str, Any]]:
    """Get stack parameters from configuration.

    Returns:
        Dictionary of stack parameters organized by namespace
    """
    config = load_pulumi_config()
    stack_params = config.get("stackParams", {})
    return stack_params if stack_params is not None else {}


def get_pulumi_log_level() -> int:
    """Get Pulumi log level from configuration.

    Returns:
        Log level (default: 3)
    """
    config = load_pulumi_config()
    pulumi_config = config.get("pulumi", {})
    return pulumi_config.get("logLevel", 3)


def get_pulumi_log_to_stderr() -> bool:
    """Get Pulumi log to stderr setting.

    Returns:
        True if logs should go to stderr
    """
    config = load_pulumi_config()
    pulumi_config = config.get("pulumi", {})
    return pulumi_config.get("logToStdErr", False)


def get_stack_name(stack_name: str | None, scenario: str) -> str:
    """Generate a stack name.

    Args:
        stack_name: Explicit stack name, or None to generate from scenario
        scenario: Scenario name (e.g., "aws/eks")

    Returns:
        Stack name to use
    """
    if stack_name:
        return stack_name

    # Generate from scenario: "aws/eks" -> "aws-eks"
    return scenario.replace("/", "-")


def get_default_workload_install() -> bool:
    """Get default workload install setting.

    Returns:
        True if workload should be installed by default
    """
    # Default to False for lab environments
    return False


def get_api_key() -> str:
    """Get Datadog API key from configuration.

    Returns:
        API key, or empty string if not configured
    """
    api_key, _ = get_agent_keys()
    return api_key


def get_app_key() -> str:
    """Get Datadog App key from configuration.

    Returns:
        App key, or empty string if not configured
    """
    _, app_key = get_agent_keys()
    return app_key


def get_pulumi_directory() -> Path:
    """Get the directory containing Pulumi program.

    Returns:
        Path to test/e2e-framework/run directory
    """
    # Navigate from this file to the pulumi program directory
    # .dda/extend/pythonpath/lab/pulumi_config.py -> ../../../../test/e2e-framework/run
    return Path(__file__).parents[4] / "test" / "e2e-framework" / "run"
