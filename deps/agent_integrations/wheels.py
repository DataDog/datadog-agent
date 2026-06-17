"""Utilities for working with wheel inputs."""

from pathlib import Path


def expand_wheel_inputs(paths: list[Path]) -> list[Path]:
    """Expand direct wheel files and wheelhouse directories into sorted wheel paths."""
    wheels = []
    for path in paths:
        if path.is_dir():
            wheels.extend(path.glob("*.whl"))
        else:
            wheels.append(path)
    return sorted(wheels)
