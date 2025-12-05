from __future__ import annotations

from itertools import batched
from typing import Any

from msgspec import Struct

from utils.ci.config.model.unit import CIUnit


class RegisteredCIUnit(Struct):
    id: str
    """The unique identifier of the unit"""

    config: CIUnit
    """The configuration of the unit that is used to generate the registry file"""

    config_file: str
    """The path to the config file relative to the repository root that defines the unit"""


def static_unit_registration(unit: RegisteredCIUnit) -> str:
    data = {
        f"unit:{unit.id}": {
            "extends": ".unit:static:trigger",
            "variables": {
                "PARENT_PIPELINE_ID": "$CI_PIPELINE_ID",
                "UNIT_ID": unit.id,
                "UNIT_DISPLAY_NAME": unit.config.name,
            },
            "trigger": {"include": [{"local": unit.config.pipeline.path}]},
            "rules": _generate_rules(unit),
        },
    }
    return _to_yaml(data)


def dynamic_unit_registration(unit: RegisteredCIUnit) -> str:
    trigger_job_name = f"unit:{unit.id}"
    generate_job_name = f"{trigger_job_name}:generate"
    data = {
        generate_job_name: {
            "extends": ".unit:dynamic:generate",
            "variables": {
                "PARENT_PIPELINE_ID": "$CI_PIPELINE_ID",
                "UNIT_ID": unit.id,
                "UNIT_DISPLAY_NAME": unit.config.name,
                "UNIT_GENERATOR_COMMAND": unit.config.pipeline.command,
            },
            "rules": _generate_rules(unit),
        },
        trigger_job_name: {
            "extends": ".unit:dynamic:trigger",
            "variables": {
                "UNIT_ID": unit.id,
                "UNIT_DISPLAY_NAME": unit.config.name,
            },
            "needs": [{"job": generate_job_name}],
            "trigger": {"include": [{"artifact": "pipeline.yml", "job": generate_job_name}]},
        },
    }
    return _to_yaml(data)


def _generate_rules(unit: RegisteredCIUnit) -> list[dict[str, Any]]:
    rules: list[dict[str, Any]] = []
    if unit.config.trigger.allow_manual:
        # Allow manual trigger of the unit via explicit unit ID or `all`
        rules.append({"if": f"$TRIGGER_UNITS =~ /(^|,)({unit.id}|all)(,|$)/"})

    for trigger in unit.config.triggers:
        if trigger.exclude is not None:
            rules.extend(
                {"when": "never", "changes": {"paths": list(paths)}} for paths in batched(sorted(trigger.exclude), 50)
            )

        if trigger.include is not None:
            include_patterns = sorted(trigger.include)
            # Ensure the config file itself is included as a trigger
            if unit.config_file not in include_patterns:
                include_patterns.append(unit.config_file)

            # Batch the paths into chunks of 50 to avoid exceeding the maximum number of paths per rule
            rules.extend({"changes": {"paths": list(paths)}} for paths in batched(include_patterns, 50))

    return rules


def _to_yaml(data: dict) -> str:
    import yaml

    return yaml.dump(data, default_flow_style=False, sort_keys=False)
