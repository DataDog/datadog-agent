from __future__ import annotations

from itertools import batched
from typing import Any

from msgspec import Struct, to_builtins

from utils.ci.config.model.unit import CIUnit
from utils.ci.config.model.unit.gitlab import DynamicGitLabPipeline, GitLabUnitProviderConfig, StaticGitLabPipeline

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


class RegisteredCIUnit(Struct):
    id: str
    """The unique identifier of the unit"""

    config: CIUnit
    """The configuration of the unit that is used to generate the registry file"""

    config_file: str
    """The path to the config file relative to the repository root that defines the unit"""


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
        f"unit:{unit.id}": {
            "extends": [".unit:static:trigger"],
            "variables": {
                "PARENT_PIPELINE_ID": "$CI_PIPELINE_ID",
                "UNIT_ID": unit.id,
                "UNIT_DISPLAY_NAME": unit.config.name,
            },
            "trigger": {"include": [{"local": unit.config.provider.pipeline.path}]},
            "rules": _gitlab_generate_rules(unit),
        },
    }
    return _generate_yaml(data)


def _gitlab_dynamic_unit_registration(unit: RegisteredCIUnit) -> str:
    trigger_job_name = f"unit:{unit.id}"
    generate_job_name = f"{trigger_job_name}:generate"
    data = {
        generate_job_name: {
            "extends": [".unit:dynamic:generate"],
            "variables": {
                "PARENT_PIPELINE_ID": "$CI_PIPELINE_ID",
                "UNIT_ID": unit.id,
                "UNIT_DISPLAY_NAME": unit.config.name,
                "UNIT_GENERATOR_COMMAND": unit.config.provider.pipeline.command,
            },
            "rules": _gitlab_generate_rules(unit),
        },
        trigger_job_name: {
            "extends": [".unit:dynamic:trigger"],
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
        patterns.append(unit.config_file)

    # Disallow tags by default
    if not unit.config.trigger.allow_tags:
        rules.append({"when": "never", "if": "$CI_COMMIT_TAG"})

    # Allow manual triggering via explicit unit ID or `all`
    if unit.config.trigger.allow_manual:
        rules.append({"if": rf"$TRIGGER_UNITS =~ /^all$|\b{unit.id}\b/"})

    # Add the unit-defined rules before change detection to allow for more control
    rules.extend(to_builtins(rule) for rule in unit.config.provider.rules)

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


def _generate_yaml(data: dict) -> str:
    import yaml

    return f"""\
# This file is automatically generated using the following command:
#
#   dda check ci units --fix
{yaml.dump(data, default_flow_style=False, sort_keys=False).rstrip()}
"""
