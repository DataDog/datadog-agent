from __future__ import annotations

from itertools import batched
from typing import TYPE_CHECKING, Any

from dda.utils.fs import Path

if TYPE_CHECKING:
    from utils.ci.config.model.unit import CIUnit


def static_unit_registration(config: CIUnit, *, source: Path) -> str:
    data = {
        config.id: {
            "extends": ".unit:static:trigger",
            "variables": {
                "PARENT_PIPELINE_ID": "$CI_PIPELINE_ID",
                "UNIT_ID": config.id,
                "UNIT_DISPLAY_NAME": config.name,
            },
            "trigger": {"include": [{"local": config.pipeline.path}], "strategy": "depend"},
            "rules": _generate_rules(source, config),
        },
    }
    return _to_yaml(data)


def dynamic_unit_registration(config: CIUnit, *, source: Path) -> str:
    trigger_job_name = f"unit:{config.id}"
    generate_job_name = f"{trigger_job_name}:generate"
    data = {
        generate_job_name: {
            "extends": ".unit:dynamic:generate",
            "variables": {
                "PARENT_PIPELINE_ID": "$CI_PIPELINE_ID",
                "UNIT_ID": config.id,
                "UNIT_DISPLAY_NAME": config.name,
                "UNIT_GENERATOR_COMMAND": config.pipeline.command,
            },
            "rules": _generate_rules(source, config),
        },
        trigger_job_name: {
            "extends": ".unit:dynamic:trigger",
            "variables": {
                "UNIT_ID": config.id,
                "UNIT_DISPLAY_NAME": config.name,
            },
            "needs": [{"job": generate_job_name}],
            "trigger": {
                "include": [{"artifact": "pipeline.yml", "job": generate_job_name}],
                "strategy": "depend",
            },
        },
    }
    return _to_yaml(data)


def _generate_rules(source: Path, config: CIUnit) -> list[dict[str, Any]]:
    # Allow manual trigger of the unit via explicit unit ID or `all`
    rules: list[dict[str, Any]] = [{"if": f"$RUN_UNITS =~ /(^|,)({config.id}|all)(,|$)/"}]
    for trigger in config.triggers:
        relative_paths = sorted(trigger.changes)
        # Ensure the config file itself is included as a trigger
        if (config_path := source.as_posix()) not in relative_paths:
            relative_paths.append(config_path)

        # Batch the paths into chunks of 50 to avoid exceeding the maximum number of paths per rule
        rules.extend({"changes": list(paths)} for paths in batched(relative_paths, 50))

    return rules


def _to_yaml(data: dict) -> str:
    import yaml

    return yaml.dump(data, default_flow_style=False, sort_keys=False)
