"""Bypass the heavyweight tasks/__init__.py.

Tests import modules as `coordinator.x` (not `tasks.coordinator.x`) by
putting the `tasks/` directory directly on sys.path before pytest resolves
any test imports. That way `coordinator` is a top-level package and we
skip running `tasks/__init__.py`.
"""

import pathlib
import sys

_TASKS_DIR = pathlib.Path(__file__).resolve().parents[2]  # .../tasks

if str(_TASKS_DIR) not in sys.path:
    sys.path.insert(0, str(_TASKS_DIR))
