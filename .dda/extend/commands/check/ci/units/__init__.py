from __future__ import annotations

from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app
from dda.utils.fs import Path

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Validate GitLab configuration",
    dependencies=["pyyaml"],
)
@click.option("--fix", is_flag=True, help="Resolve issues")
@pass_app
def cmd(app: Application, *, fix: bool) -> None:
    import tomllib

    import msgspec
    from utils.ci.config.model.unit import CIDynamicPipeline, CIStaticPipeline, CIUnit
    from utils.ci.units import RegisteredCIUnit, dynamic_unit_registration, static_unit_registration

    # json_schema = msgspec.json.schema(CIUnit)
    # print(json_schema)
    # return

    config_dir = Path("ci", "units")
    registry_dir = Path(".ci", "units")
    configured_units: set[str] = set()
    unrecoverable_errors = 0
    fixable_errors = 0
    for config_file in config_dir.glob("*.toml"):
        unit_id = config_file.stem
        configured_units.add(unit_id)
        try:
            data = tomllib.loads(config_file.read_text(encoding="utf-8"))
        except Exception as e:
            app.display_error(f"Unable to parse CI unit config: {config_file}\n{e}")
            unrecoverable_errors += 1
            continue

        try:
            config = msgspec.convert(data, CIUnit)
        except Exception as e:
            app.display_error(f"Invalid CI unit config: {config_file}\n{e}")
            unrecoverable_errors += 1
            continue

        unit = RegisteredCIUnit(id=unit_id, config=config, config_file=config_file.as_posix())

        registry_file = registry_dir / f"{unit_id}.yml"
        try:
            registry_file_contents = registry_file.read_text(encoding="utf-8")
        except Exception:
            registry_file_contents = ""

        if isinstance(unit.config.pipeline, CIStaticPipeline):
            expected_registry_file_contents = static_unit_registration(unit)
        elif isinstance(unit.config.pipeline, CIDynamicPipeline):
            expected_registry_file_contents = dynamic_unit_registration(unit)
        else:
            app.display_error(f"Invalid CI unit pipeline type `{type(unit.config.pipeline)}`: {unit.config_file}")
            unrecoverable_errors += 1
            continue

        if registry_file_contents != expected_registry_file_contents:
            fixable_errors += 1
            if fix:
                registry_file.write_text(expected_registry_file_contents, encoding="utf-8")
                app.display_success(f"Updated CI unit registration: {registry_file}")
                fixable_errors -= 1
            else:
                app.display_error(f"CI unit registration out of sync with config: {registry_file}")

    for registry_file in registry_dir.glob("*.yml"):
        unit_id = registry_file.stem
        if unit_id not in configured_units:
            fixable_errors += 1
            if fix:
                registry_file.unlink()
                app.display_success(f"Removed unregistered CI unit: {registry_file}")
                fixable_errors -= 1
            else:
                app.display_error(f"Unregistered CI unit: {unit_id}")

    if unrecoverable_errors or fixable_errors:
        if fixable_errors and not fix:
            app.display_warning("Run with --fix to resolve the issues")

        app.abort()
