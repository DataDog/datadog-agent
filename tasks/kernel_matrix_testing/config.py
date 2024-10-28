from __future__ import annotations

import json
from typing import TYPE_CHECKING, cast

from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import Exit

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import KMTConfig


class ConfigManager:
    def __init__(self):
        self._cfg_path = get_kmt_os().kmt_dir / "config.json"
        self._config: KMTConfig | None = None

    def load(self):
        if not self._cfg_path.is_file():
            self._config = cast('KMTConfig', {})
        else:
            with open(self._cfg_path) as f:
                self._config = json.load(f)

    @property
    def config(self) -> KMTConfig:
        if self._config is None:
            self.load()
        if self._config is None:  # Check in case of failure, also for typing
            raise Exit("Could not load config")
        return self._config

    def save(self):
        # Serialize to JSON before writing to avoid leaving an empty file if JSON serialization
        # fails
        json_data = json.dumps(self._config, indent=2)
        with open(self._cfg_path, "w") as f:
            f.write(json_data)
