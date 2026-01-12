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
    import re

    from utils.ci.units import RegisteredCIUnit, unit_registration

    root_config_dir = Path("ci", "units")
    registry_dir = Path(".ci", "units")
    configured_units: set[str] = set()
    unrecoverable_errors = 0
    fixable_errors = 0
    for config_dir in root_config_dir.iterdir():
        unit = RegisteredCIUnit(config_dir)
        configured_units.add(unit.id)
        try:
            registry_file = registry_dir / unit.provider_name / f"{unit.id}.yml"
        except Exception as e:
            app.display_error(f"Unable to load CI unit config: {unit.config_file}\n{e}")
            unrecoverable_errors += 1
            continue

        try:
            registry_file_contents = registry_file.read_text(encoding="utf-8")
        except Exception:
            registry_file_contents = ""

        try:
            expected_registry_file_contents = unit_registration(unit)
        except Exception as e:
            app.display_error(f"Unable to generate CI unit registration: {unit.config_file}\n{e}")
            unrecoverable_errors += 1
            continue

        if registry_file_contents != expected_registry_file_contents:
            fixable_errors += 1
            if fix:
                registry_file.parent.ensure_dir()
                registry_file.write_text(expected_registry_file_contents, encoding="utf-8")
                app.display_success(f"Updated CI unit registration: {registry_file}")
                fixable_errors -= 1
            else:
                app.display_error(f"CI unit registration out of sync with config: {registry_file}")

    for provider_registry_dir in registry_dir.iterdir():
        for registry_file in provider_registry_dir.glob("*.yml"):
            unit_id = registry_file.stem
            if unit_id not in configured_units:
                fixable_errors += 1
                if fix:
                    registry_file.unlink()
                    app.display_success(f"Removed unregistered CI unit: {registry_file}")
                    fixable_errors -= 1
                else:
                    app.display_error(f"Unregistered CI unit: {unit_id}")

    root_pipeline_file = Path(".gitlab-ci.yml")
    root_pipeline_contents = root_pipeline_file.read_text(encoding="utf-8")
    unit_regex_pattern = re.compile(
        r"""
        ^spec:[ ]*$\n  # Start of `spec` block
        (?:^(|[ ]+.*)$\n)*?  # Any blank or indented lines
        ^(?P<indent>[ ]+)inputs:[ ]*$\n  # Start of `spec.inputs` block
        (?:^(|(?P=indent){2}.+)$\n)*?  # Any blank lines or indented lines in the `spec.inputs` block
        ^(?P=indent){2}units:[ ]*$\n  # Start of `spec.inputs.units` block
        (?:^(|(?P=indent){3}.+)$\n)*?  # Any blank lines or indented lines in the `spec.inputs.units` block
        ^(?P=indent){3}regex:[ ](?P<regex>[^\r\n]*)  # The `regex` field for the `spec.inputs.units` input
        """,
        flags=re.VERBOSE | re.MULTILINE,
    )
    match = unit_regex_pattern.search(root_pipeline_contents)
    if not match:
        app.display_error(f"Missing the `regex` field for the `units` input: {root_pipeline_file}")
        unrecoverable_errors += 1
    else:
        unit_ids = "|".join(sorted(configured_units))
        expected_regex = rf"^(all|{unit_ids}(,{unit_ids})*)$"
        if (regex := match.group("regex")) != expected_regex:
            fixable_errors += 1
            if fix:
                regex_start, regex_end = match.span("regex")
                root_pipeline_file.write_text(
                    root_pipeline_contents[:regex_start] + expected_regex + root_pipeline_contents[regex_end:],
                    encoding="utf-8",
                )
                app.display_success(f"Updated `regex` field for the `units` input: {root_pipeline_file}")
                fixable_errors -= 1
            else:
                app.display_error(f"Incorrect `regex` field for the `units` input: {root_pipeline_file}")
                app.display_error(f"Expected: {expected_regex}")
                app.display_error(f"Actual: {regex}")

    if unrecoverable_errors or fixable_errors:
        if fixable_errors and not fix:
            app.display_warning("Run with --fix to resolve the issues")

        app.abort()
