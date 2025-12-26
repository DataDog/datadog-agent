# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Lab environment provider system with automatic command generation.

Providers register themselves and automatically get CLI commands.
Environment storage is handled automatically by the framework.

Example provider definition:

    @register_provider
    class KindProvider(BaseProvider):
        name = "kind"
        category = "local"  # -> dda lab local kind
        description = "Local Kind cluster"

        create_options = [
            Option("--k8s-version", default="v1.31.0", help="Kubernetes version"),
        ]

        def create(self, app, config) -> dict[str, Any] | None:
            # Create the resource
            # return metadata about what was created
            ...

        def destroy(self, app, name, *, force=False):
            # Destroy the resource
            ...
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any, Literal

if TYPE_CHECKING:
    from click import Option
    from dda.cli.application import Application

    from lab.config import LabConfig

ProviderAction = Literal["create", "delete"]


@dataclass
class MissingPrerequisite:
    """A missing prerequisite with remediation instructions."""

    name: str
    """What is missing (e.g., 'kind', 'helm', 'developer environment')."""

    remediation: str
    """How to fix it (e.g., 'brew install kind', 'dda env dev start')."""

    actions: set[ProviderAction] = field(default_factory=lambda: {"create", "delete"})
    """Actions this prerequisite should be checked for."""

    def __str__(self) -> str:
        return f"{self.name} ({self.remediation})"


@dataclass
class ProviderConfig:
    """Raw configuration passed to provider for options parsing."""

    name: str
    options: dict[str, Any] = field(default_factory=dict)
    lab_config: LabConfig = None  # LabConfig instance, injected by command framework

    def get(self, key: str, default: Any = None) -> Any:
        return self.options.get(key, default)

    def get_api_key(self) -> str | None:
        """Get API key from options, lab config, or environment."""
        # First check if explicitly passed as option
        if api_key := self.get("api_key"):
            return api_key
        # Then try lab config
        if self.lab_config:
            return self.lab_config.get_api_key()
        return None

    def get_app_key(self) -> str | None:
        """Get APP key from options, lab config, or environment."""
        # First check if explicitly passed as option
        if app_key := self.get("app_key"):
            return app_key
        # Then try lab config
        if self.lab_config:
            return self.lab_config.get_app_key()
        return None


@dataclass
class ProviderOptions:
    """Base class for typed provider options."""

    name: str

    @classmethod
    def from_config(cls, config: ProviderConfig) -> ProviderOptions:
        """Create options from ProviderConfig. Override in subclasses."""
        return cls(name=config.name)


class BaseProvider(ABC):
    """
    Base class for lab environment providers.

    Subclasses define:
    - name: unique identifier (e.g., "kind", "gke")
    - category: command grouping (e.g., "local", "gcp", "aws")
    - description: human-readable description
    - create_options: list of Option for create command
    - options_class: typed options dataclass for this provider

    Environment storage is handled automatically by the framework:
    - After successful create(): environment is saved with returned metadata
    - After successful destroy(): environment is removed (via dda lab delete)
    """

    # Required class attributes (set in subclasses)
    name: str
    category: str
    description: str

    # CLI options for create command (override in subclasses)
    create_options: list[Option] = []

    # Typed options class for this provider (override in subclasses)
    options_class: type[ProviderOptions] = ProviderOptions

    @abstractmethod
    def create(self, app: Application, options: ProviderOptions) -> dict[str, Any] | None:
        """
        Create the environment.

        Args:
            app: Application instance
            options: Typed options for this provider (subclass of ProviderOptions)

        Returns:
            Optional dict of metadata about what was created (IPs, endpoints, etc.)
            This metadata is stored and shown in `dda lab list`.

        Example:
            return {
                "ip": "192.168.1.100",
                "endpoint": "https://my-cluster.example.com",
                "node_count": 3,
            }
        """
        ...

    @abstractmethod
    def destroy(self, app: Application, name: str) -> None:
        """Destroy the environment. Storage cleanup is handled by dda lab delete."""
        ...

    def check_prerequisites(self, app: Application, options: ProviderOptions) -> list[MissingPrerequisite]:
        """Return list of missing prerequisites. Each prerequisite declares when it applies."""
        return []


# Provider registry
_PROVIDERS: dict[str, type[BaseProvider]] = {}
_BUILTINS_LOADED = False


def register_provider(cls: type[BaseProvider]) -> type[BaseProvider]:
    """Decorator to register a provider class."""
    _PROVIDERS[cls.name] = cls
    return cls


def get_provider(name: str) -> BaseProvider:
    """Get a provider instance by name."""
    if name not in _PROVIDERS:
        available = ", ".join(sorted(_PROVIDERS.keys()))
        raise ValueError(f"Unknown provider '{name}'. Available: {available}")
    return _PROVIDERS[name]()


def list_providers() -> list[type[BaseProvider]]:
    """List all registered provider classes."""
    return list(_PROVIDERS.values())


def get_providers_by_category() -> dict[str, list[type[BaseProvider]]]:
    """Group providers by category."""
    by_category: dict[str, list[type[BaseProvider]]] = {}
    for provider_cls in _PROVIDERS.values():
        category = provider_cls.category
        by_category.setdefault(category, []).append(provider_cls)
    return by_category
