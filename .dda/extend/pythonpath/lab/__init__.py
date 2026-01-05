# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT

from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from dda.cli.application import Application


ENVIRONMENTS_DIR = "environments"


# =============================================================================
# Environment Storage - One file per environment
# =============================================================================


def _get_environments_dir(app: Application) -> Path:
    """Get the environments storage directory.

    Each environment is stored as a separate JSON file.
    Structure: ~/.local/share/dda/lab/environments/{name}.json
    """
    storage_dir = Path(app.config.storage.join("lab").data) / ENVIRONMENTS_DIR
    storage_dir.mkdir(parents=True, exist_ok=True)
    return storage_dir


def _get_environment_path(app: Application, name: str) -> Path:
    """Get the path to a specific environment's JSON file."""
    return _get_environments_dir(app) / f"{name}.json"


@dataclass
class LabEnvironment:
    """
    Generic lab environment that can represent any type of environment.

    Attributes:
        app: Application instance for storage access
        name: Unique identifier for the environment
        category: Category of the environment (e.g., "local", "aws", "gcp")
        env_type: Type of environment (e.g., "kind", "gke", "eks", "minikube")
        created_at: ISO timestamp when the environment was created
        metadata: Type-specific metadata (flexible dict for provider-specific data)
    """

    app: Application = field(repr=False, compare=False)
    name: str
    env_type: str
    category: str
    created_at: str = ""
    metadata: dict[str, Any] = field(default_factory=dict)

    def __post_init__(self) -> None:
        if not self.created_at:
            self.created_at = datetime.now().isoformat()

    @classmethod
    def from_dict(cls, app: Application, data: dict[str, Any]) -> LabEnvironment:
        """Create a LabEnvironment from a dictionary."""
        return cls(
            app=app,
            name=data["name"],
            category=data["category"],
            env_type=data["env_type"],
            created_at=data.get("created_at", "unknown"),
            metadata=data.get("metadata", {}),
        )

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary (excludes app)."""
        return {
            "name": self.name,
            "category": self.category,
            "env_type": self.env_type,
            "created_at": self.created_at,
            "metadata": self.metadata,
        }

    def save(self) -> None:
        """Save this environment to its own file."""
        path = _get_environment_path(self.app, self.name)
        with open(path, "w") as f:
            json.dump(self.to_dict(), f, indent=2)

    def delete(self):
        """Delete this environment's file. Returns True if found and removed."""
        path = _get_environment_path(self.app, self.name)
        if path.exists():
            path.unlink()
            return True
        raise FileNotFoundError(f"Environment '{self.name}' not found")

    @classmethod
    def load(cls, app: Application, name: str) -> LabEnvironment | None:
        """Load a specific environment by name."""
        path = _get_environment_path(app, name)
        if not path.exists():
            return None
        with open(path) as f:
            data = json.load(f)
            return cls.from_dict(app, data)

    @classmethod
    def load_all(cls, app: Application, env_type: str | None = None) -> list[LabEnvironment]:
        """Load all stored environments, optionally filtered by type."""
        envs_dir = _get_environments_dir(app)
        envs: list[LabEnvironment] = []
        for path in envs_dir.glob("*.json"):
            try:
                with open(path) as f:
                    data = json.load(f)
                    env = cls.from_dict(app, data)
                    if env_type is None or env.env_type == env_type:
                        envs.append(env)
            except (json.JSONDecodeError, KeyError):
                # Skip invalid files
                continue
        return envs

    @classmethod
    def exists(cls, app: Application, name: str) -> bool:
        """Check if an environment with this name exists."""
        return _get_environment_path(app, name).exists()
