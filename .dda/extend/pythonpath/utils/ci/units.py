from __future__ import annotations

import re
import tomllib
from functools import cached_property
from itertools import batched
from typing import TYPE_CHECKING, Any

import msgspec

from utils.ci.config.model.unit import CIUnit
from utils.ci.config.model.unit.gitlab import DynamicGitLabPipeline, GitLabUnitProviderConfig, StaticGitLabPipeline

if TYPE_CHECKING:
    from dda.utils.fs import Path

# The maximum number of paths per rule in GitLab CI
MAX_PATHS_PER_RULE = 50
# CI infra injects this variable that represents the target branch of PRs or `main` otherwise
GIT_BASE_BRANCH = "$GIT_BASE_BRANCH"
# These patterns match long-lived branches that PRs typically target
STABLE_BRANCH_PATTERNS = (
    # Default branch
    "main",
    # Release branches
    r"[0-9]+\.[0-9]+\.x",
)


class RegisteredCIUnit:
    def __init__(self, config_dir: Path) -> None:
        self.__config_dir = config_dir

    @property
    def config_dir(self) -> Path:
        return self.__config_dir

    @cached_property
    def config_file(self) -> Path:
        return self.config_dir / "config.toml"

    @property
    def id(self) -> str:
        return self.config_dir.name

    @cached_property
    def config(self) -> CIUnit:
        data = tomllib.loads(self.config_file.read_text(encoding="utf-8"))
        return msgspec.convert(data, CIUnit)

    @cached_property
    def provider_name(self) -> str:
        return msgspec.inspect.type_info(type(self.config.provider)).tag


def unit_registration(unit: RegisteredCIUnit) -> str:
    if isinstance(unit.config.provider, GitLabUnitProviderConfig):
        if isinstance(unit.config.provider.pipeline, StaticGitLabPipeline):
            return _gitlab_static_unit_registration(unit)
        elif isinstance(unit.config.provider.pipeline, DynamicGitLabPipeline):
            return _gitlab_dynamic_unit_registration(unit)
        else:
            msg = f"Invalid GitLab pipeline type: {type(unit.config.provider.pipeline)}"
            raise ValueError(msg)

    msg = f"Invalid unit provider: {type(unit.config.provider)}"
    raise ValueError(msg)


def _gitlab_static_unit_registration(unit: RegisteredCIUnit) -> str:
    data = {
        _unit_id_as_job_name(unit.id): {
            "extends": [".job:unit:base", ".job:unit:static:trigger"],
            "variables": {
                "UNIT_ID": unit.id,
                "UNIT_DISPLAY_NAME": unit.config.name,
            },
            "trigger": {"include": [{"local": unit.config.provider.pipeline.path}]},
            "rules": _gitlab_generate_rules(unit),
        },
    }
    return _generate_yaml(data)


def _gitlab_dynamic_unit_registration(unit: RegisteredCIUnit) -> str:
    trigger_job_name = _unit_id_as_job_name(unit.id)
    generate_job_name = f"{trigger_job_name}:generate"
    data = {
        generate_job_name: {
            "extends": [".job:unit:dynamic:generate"],
            "variables": {
                "UNIT_ID": unit.id,
                "UNIT_DISPLAY_NAME": unit.config.name,
                "UNIT_GENERATOR_COMMAND": unit.config.provider.pipeline.command,
            },
            "rules": _gitlab_generate_rules(unit),
        },
        trigger_job_name: {
            "extends": [".job:unit:base", ".job:unit:dynamic:trigger"],
            "variables": {
                "UNIT_ID": unit.id,
                "UNIT_DISPLAY_NAME": unit.config.name,
            },
            "needs": [{"job": generate_job_name}],
            "trigger": {"include": [{"artifact": "pipeline.yml", "job": generate_job_name}]},
        },
    }
    return _generate_yaml(data)


def _gitlab_generate_rules(unit: RegisteredCIUnit) -> list[dict[str, Any]]:
    rules: list[dict[str, Any]] = []

    patterns = sorted(
        unit.config.trigger.patterns,
        # Sort patterns by the number of wildcards descending, then case-insensitively
        key=lambda s: (-s.count("**"), -s.count("*"), s.casefold()),
    )
    if unit.config.trigger.watch_config and unit.config_file not in patterns:
        patterns.append(unit.config_file.as_posix())

    # Allow manual triggering via explicit unit ID or `all`
    if unit.config.trigger.allow_manual:
        rules.extend(
            (
                {"if": rf"$IS_UNIT_SELECTION && $TRIGGER_UNITS =~ /^all$|\b{re.escape(unit.id)}\b/"},
                # Skip if units were selectable and the unit was not included
                {"if": "$IS_UNIT_SELECTION", "when": "never"},
            )
        )

    # Add the unit-defined rules before change detection to allow for more control
    rules.extend(msgspec.to_builtins(rule) for rule in unit.config.provider.rules)

    # Add the change detection rules if there are any patterns to watch
    if patterns:
        # Construct the GitLab rules for the unit's triggers, ensuring that all patterns are
        # batched into chunks of 50 to avoid exceeding the maximum number of paths per rule
        path_batches = list(map(list, batched(patterns, MAX_PATHS_PER_RULE)))

        # There are 3 different change detection scenarios:
        #
        # 1. On PRs, we want to compare changes to the target branch.
        # 2. On commits to long-lived branches (e.g. main, release branches), we want to
        #    compare changes to the previous commit.
        # 3. On commits to any other branch, we want to compare changes to the `main` branch
        #    because it is by far the most common target branch of PRs.
        #
        # The environment variable injected by CI infra satisfies scenario 1 and 3.
        # In order to satisfy scenario 2, we omit the `compare_to` field to use
        # the default behavior of comparing changes to the previous commit.
        rules.extend(
            {"if": f"$CI_COMMIT_BRANCH =~ /^({'|'.join(STABLE_BRANCH_PATTERNS)})$/", "changes": paths}
            for paths in path_batches
        )
        rules.extend({"changes": {"compare_to": GIT_BASE_BRANCH, "paths": paths}} for paths in path_batches)

    return rules


def _unit_id_as_job_name(unit_id: str) -> str:
    # Support hierarchies via dot separators
    return f"unit:{unit_id.replace(".", ":")}"


def _generate_yaml(data: dict) -> str:
    import yaml

    return f"""\
# This file is automatically generated using the following command:
#
#   dda check ci units --fix
{yaml.dump(data, default_flow_style=False, sort_keys=False).rstrip()}
"""
