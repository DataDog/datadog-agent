#!/usr/bin/env python3
"""Resolve /follow-pr autonomy preferences from config files, env vars, and flags.

Precedence, lowest to highest:
  built-in defaults < global TOML < worktree-local TOML < environment < CLI flags

Global TOML lives beside the file `dda config find` reports, as `follow-pr.toml`.
Worktree-local TOML lives at `<repo-root>/.agents/skills/follow-pr/config.toml`
(gitignored, so each worktree/checkout can carry its own preference).

Usage:
    config.py resolve [--mode autofix|no-autofix|ask]
                       [--max-fix-cycles N]
                       [--policy TEXT]

Prints the resolved values as plain text, one field per line, each tagged
with the layer that supplied it (default/global/local/environment/explicit).
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
import tomllib
from collections import ChainMap
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from typing import Any

FIELDS = ("mode", "max_fix_cycles", "policy")

ENV_VARS = {
    "mode": "DDA_FOLLOW_PR_MODE",
    "max_fix_cycles": "DDA_FOLLOW_PR_MAX_FIX_CYCLES",
    "policy": "DDA_FOLLOW_PR_POLICY",
}


class FixMode(str, Enum):
    AUTOFIX = "autofix"
    NO_AUTOFIX = "no-autofix"
    ASK = "ask"


DEFAULTS: dict[str, Any] = {"mode": FixMode.AUTOFIX.value, "max_fix_cycles": 2, "policy": ""}


@dataclass(frozen=True)
class ResolvedConfig:
    mode: str
    max_fix_cycles: int
    policy: str
    sources: dict[str, str]

    def render(self) -> str:
        lines = [
            f"mode: {self.mode} (source: {self.sources['mode']})",
            f"max_fix_cycles: {self.max_fix_cycles} (source: {self.sources['max_fix_cycles']})",
        ]
        if self.policy:
            lines.append(f"policy (source: {self.sources['policy']}):")
            lines.append(self.policy)
        else:
            lines.append(f"policy: none (source: {self.sources['policy']})")
        return "\n".join(lines)


def _run(cmd: list[str]) -> str | None:
    """Run `cmd` and return its stdout, or None if it couldn't tell us anything.

    Both call sites use this only to *locate* an optional config layer (the
    `dda` global config file, or this repo's checkout root). Any failure here
    — the binary isn't installed, or it ran but errored (a locked/uninitialized
    `dda` config, a sandbox that blocks it, `git` failing outside a checkout)
    — means that layer simply isn't available, not that resolution overall
    should fail. A malformed config file that *was* found is a different,
    louder failure, raised separately in `_load_toml`.
    """
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
    except (OSError, subprocess.CalledProcessError):
        return None
    return result.stdout.strip() or None


def _global_config_path() -> Path | None:
    dda_config = _run(["dda", "config", "find"])
    if not dda_config:
        return None
    return Path(dda_config).expanduser().parent / "follow-pr.toml"


def _local_config_path() -> Path | None:
    toplevel = _run(["git", "rev-parse", "--show-toplevel"])
    if not toplevel:
        return None
    return Path(toplevel) / ".agents" / "skills" / "follow-pr" / "config.toml"


def _load_toml(path: Path | None, source: str) -> dict[str, Any]:
    if path is None or not path.is_file():
        return {}
    with path.open("rb") as f:
        try:
            data = tomllib.load(f)
        except tomllib.TOMLDecodeError as exc:
            raise ValueError(f"{source}: invalid TOML in {path}: {exc}") from exc
    return {name: value for name, value in data.items() if name in FIELDS}


def _env_layer() -> dict[str, Any]:
    layer: dict[str, Any] = {}
    if os.environ.get(ENV_VARS["mode"]) is not None:
        layer["mode"] = os.environ[ENV_VARS["mode"]]
    if os.environ.get(ENV_VARS["max_fix_cycles"]) is not None:
        raw = os.environ[ENV_VARS["max_fix_cycles"]]
        try:
            layer["max_fix_cycles"] = int(raw)
        except ValueError:
            raise ValueError(f"environment: max_fix_cycles must be an integer, got {raw!r}") from None
    if os.environ.get(ENV_VARS["policy"]) is not None:
        layer["policy"] = os.environ[ENV_VARS["policy"]]
    return layer


def _validate_mode(source: str, value: Any) -> str:
    try:
        return FixMode(value).value
    except ValueError:
        raise ValueError(f"{source}: mode must be one of {[m.value for m in FixMode]}, got {value!r}") from None


def _validate(source: str, name: str, value: Any) -> Any:
    if name == "mode":
        return _validate_mode(source, value)
    if name == "max_fix_cycles":
        if isinstance(value, bool) or not isinstance(value, int) or value < 1:
            raise ValueError(f"{source}: max_fix_cycles must be a positive integer, got {value!r}")
        return value
    if name == "policy":
        if not isinstance(value, str):
            raise ValueError(f"{source}: policy must be a string, got {value!r}")
        return value
    raise AssertionError(f"unknown field {name}")


def resolve(args: argparse.Namespace) -> ResolvedConfig:
    cli_layer = {
        name: value
        for name, value in (("mode", args.mode), ("max_fix_cycles", args.max_fix_cycles), ("policy", args.policy))
        if value is not None
    }

    layers: list[tuple[str, dict[str, Any]]] = [
        ("explicit", cli_layer),
        ("environment", _env_layer()),
        ("local", _load_toml(_local_config_path(), "local")),
        ("global", _load_toml(_global_config_path(), "global")),
        ("default", DEFAULTS),
    ]
    merged = ChainMap(*(layer for _, layer in layers))

    resolved: dict[str, Any] = {}
    sources: dict[str, str] = {}
    for name in FIELDS:
        source = next(source_name for source_name, layer in layers if name in layer)
        resolved[name] = _validate(source, name, merged[name])
        sources[name] = source

    return ResolvedConfig(
        mode=resolved["mode"], max_fix_cycles=resolved["max_fix_cycles"], policy=resolved["policy"], sources=sources
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    subparsers = parser.add_subparsers(dest="command", required=True)

    resolve_parser = subparsers.add_parser("resolve", help="Resolve and print the effective preferences")
    resolve_parser.add_argument("--mode", choices=[m.value for m in FixMode], default=None)
    resolve_parser.add_argument("--max-fix-cycles", dest="max_fix_cycles", type=int, default=None)
    resolve_parser.add_argument("--policy", default=None)

    args = parser.parse_args(argv)

    try:
        config = resolve(args)
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print(config.render())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
