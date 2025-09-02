"""
Configuration management for experimental static quality gates.

This module handles loading and parsing of quality gate configurations
from YAML files, providing a clean interface for accessing gate settings.
"""

from typing import Any

import yaml

from tasks.static_quality_gates.gates import QualityGateConfig, create_quality_gate_config


class ConfigurationManager:
    """Shared configuration management for all artifact measurers."""

    def __init__(self, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize configuration manager.

        Args:
            config_path: Path to the quality gates configuration file
        """
        self.config_path = config_path
        self.config = self._load_config()

    def _load_config(self) -> dict[str, Any]:
        """Load quality gates configuration from YAML file."""
        try:
            with open(self.config_path) as f:
                return yaml.safe_load(f)
        except FileNotFoundError:
            raise ValueError(f"Configuration file not found: {self.config_path}") from None
        except yaml.YAMLError as e:
            raise ValueError(f"Invalid YAML configuration: {e}") from e

    def get_gate_config(self, gate_name: str) -> QualityGateConfig:
        """Get configuration for a specific gate."""
        if gate_name not in self.config:
            raise ValueError(f"Gate configuration not found: {gate_name}")
        return create_quality_gate_config(gate_name, self.config[gate_name])
