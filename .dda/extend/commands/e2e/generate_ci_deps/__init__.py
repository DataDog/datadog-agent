from __future__ import annotations

import re
from pathlib import Path
from typing import TYPE_CHECKING

import click
import yaml
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Generate GitLab CI artifact dependencies for E2E tests",
    dependencies=[
        "pyyaml>=6.0",
        "ruamel.yaml>=0.17.0",
    ],
)
@click.option("--area", help="Generate only for specific test area (e.g., 'containers')")
@click.option("--validate", is_flag=True, help="Validate existing CI config against manifests (no write)")
@click.option("--dry-run", is_flag=True, help="Show what would be generated without writing")
@click.option("--verbose", is_flag=True, help="Show detailed generation process")
@pass_app
def cmd(app: Application, *, area: str | None, validate: bool, dry_run: bool, verbose: bool) -> None:
    """
    Generate GitLab CI artifact dependencies for E2E tests.

    Reads e2e-dependencies.yaml files from test areas and generates the
    appropriate 'needs:' sections in .gitlab/test/e2e/e2e.yml.

    Each test area can define:
      - default_artifacts: Used by all tests unless overridden
      - test_specific: Pattern-based overrides matching test names

    Examples:

        # Generate CI dependencies for all test areas
        dda e2e generate-ci-deps

        # Validate that CI config matches dependency manifests
        dda e2e generate-ci-deps --validate

        # Preview changes without writing
        dda e2e generate-ci-deps --dry-run --verbose

        # Generate only for containers test area
        dda e2e generate-ci-deps --area containers
    """
    from ruamel.yaml import YAML

    repo_root = Path.cwd()
    test_root = repo_root / "test" / "new-e2e" / "tests"
    gitlab_ci_path = repo_root / ".gitlab" / "test" / "e2e" / "e2e.yml"

    if not test_root.exists():
        app.display_error(f"Test directory not found: {test_root}")
        raise SystemExit(1)

    if not gitlab_ci_path.exists():
        app.display_error(f"GitLab CI file not found: {gitlab_ci_path}")
        raise SystemExit(1)

    # Scan for dependency manifests
    app.display("🔍 Scanning for e2e-dependencies.yaml files...")
    manifests = find_dependency_manifests(test_root, area)

    if not manifests:
        if area:
            app.display_warning(f"No e2e-dependencies.yaml found for area: {area}")
        else:
            app.display_info("No e2e-dependencies.yaml files found in any test area.")
        return

    for manifest_path in manifests:
        rel_path = manifest_path.relative_to(repo_root)
        app.display(f"  ✓ Found: {rel_path}")

    if verbose:
        app.display("")

    # Parse GitLab CI YAML
    yaml_handler = YAML()
    yaml_handler.preserve_quotes = True
    yaml_handler.default_flow_style = False

    with open(gitlab_ci_path) as f:
        ci_config = yaml_handler.load(f)

    if ci_config is None:
        app.display_error("Failed to parse GitLab CI YAML")
        raise SystemExit(1)

    # Load all manifests
    area_manifests = {}
    for manifest_path in manifests:
        with open(manifest_path) as f:
            manifest = yaml.safe_load(f)
            area_name = manifest.get("area", manifest_path.parent.name)
            area_manifests[area_name] = manifest

    # Process E2E jobs
    changes_made = False
    validation_errors = []
    jobs_processed = 0

    for job_name, job_config in ci_config.items():
        if not isinstance(job_config, dict):
            continue

        # Only process E2E test jobs
        if not job_name.startswith("new-e2e-"):
            continue

        variables = job_config.get("variables", {})
        targets = variables.get("TARGETS", "")
        extra_params = variables.get("EXTRA_PARAMS", "")

        if not targets:
            continue

        # Skip init jobs - they manage their own dependencies
        is_init = (
            job_name.endswith("-init")
            or variables.get("E2E_INIT_ONLY") == "true"
            or job_config.get("stage") == "e2e_init"
        )
        if is_init:
            continue

        # Extract test area from TARGETS (e.g., "./tests/containers" -> "containers")
        test_area = extract_test_area(targets)
        if not test_area or test_area not in area_manifests:
            continue

        # Extract test pattern from EXTRA_PARAMS --run flag
        test_pattern = extract_test_pattern(extra_params)

        manifest = area_manifests[test_area]
        expected_artifacts = determine_artifacts(manifest, test_pattern)

        # Get current artifacts from needs (resolve template inheritance)
        current_artifacts = resolve_job_artifacts(job_config, ci_config)

        if verbose:
            app.display(f"\n📋 Processing: {job_name}")
            app.display(f"   Area: {test_area}")
            if test_pattern:
                app.display(f"   Pattern: {test_pattern}")
            app.display(f"   Expected: {expected_artifacts}")
            app.display(f"   Current: {current_artifacts}")

        # Check if changes are needed
        if set(expected_artifacts) != set(current_artifacts):
            if validate:
                jobs_processed += 1
                validation_errors.append(
                    {
                        "job": job_name,
                        "area": test_area,
                        "expected": expected_artifacts,
                        "actual": current_artifacts,
                    }
                )
            elif dry_run or not validate:
                changes_made = True
                if not dry_run:
                    # Update the needs section (returns True if actually updated)
                    was_updated = update_job_needs(job_config, expected_artifacts, ci_config)
                    if was_updated:
                        jobs_processed += 1
                        if verbose:
                            app.display("   ✓ Updated needs")
                    elif verbose:
                        app.display("   ⏭️  Skipped (template-only job)")
                else:
                    app.display(f"\n  Job: {job_name}")
                    if test_pattern:
                        app.display(f"    Pattern match: {test_pattern}")
                    added = set(expected_artifacts) - set(current_artifacts)
                    removed = set(current_artifacts) - set(expected_artifacts)
                    if added:
                        app.display(f"    [ADD] Would add: {', '.join(sorted(added))}")
                    if removed:
                        app.display(f"    [REMOVE] Would remove: {', '.join(sorted(removed))}")

    # Report results
    app.display("")

    if validate:
        if validation_errors:
            app.display_error(f"❌ Validation failed! Found {len(validation_errors)} mismatch(es):\n")
            for error in validation_errors:
                app.display(f"{error['area']} area:")
                app.display(f"  Job: {error['job']}")
                app.display(f"    Expected: {error['expected']}")
                app.display(f"    Actual:   {error['actual']}")
                app.display(f"    → Run 'dda e2e generate-ci-deps --area {error['area']}' to fix\n")
            raise SystemExit(1)
        else:
            app.display_success("✅ All E2E jobs have correct artifact dependencies")
            app.display_info(f"   Validated {jobs_processed} job(s)")
    elif dry_run:
        if changes_made:
            app.display_warning(f"📝 Would update {gitlab_ci_path.name}")
            app.display_info(f"   {jobs_processed} job(s) would be modified")
            app.display("")
            app.display("✨ Run without --dry-run to apply changes")
        else:
            app.display_success("✅ No changes needed")
    else:
        if changes_made:
            # Write back the updated CI config
            with open(gitlab_ci_path, "w") as f:
                yaml_handler.dump(ci_config, f)
            app.display_success(f"✅ Updated {gitlab_ci_path.name}")
            app.display_info(f"   Modified {jobs_processed} job(s)")
        else:
            app.display_success("✅ No changes needed")


def find_dependency_manifests(test_root: Path, area_filter: str | None) -> list[Path]:
    """Find all e2e-dependencies.yaml files in test areas."""
    manifests = []

    if area_filter:
        # Check specific area
        manifest_path = test_root / area_filter / "e2e-dependencies.yaml"
        if manifest_path.exists():
            manifests.append(manifest_path)
    else:
        # Scan all test areas
        for area_dir in test_root.iterdir():
            if not area_dir.is_dir():
                continue
            manifest_path = area_dir / "e2e-dependencies.yaml"
            if manifest_path.exists():
                manifests.append(manifest_path)

    return sorted(manifests)


def extract_test_area(targets: str) -> str | None:
    """Extract test area from TARGETS variable.

    Examples:
        "./tests/containers" -> "containers"
        "./tests/windows" -> "windows"
    """
    match = re.search(r"\.?/tests/([^/\s]+)", targets)
    return match.group(1) if match else None


def extract_test_pattern(extra_params: str) -> str | None:
    """Extract test pattern from EXTRA_PARAMS --run flag.

    Examples:
        "--run TestKindSuite" -> "TestKindSuite"
        "--run 'TestDockerSuite|TestECS'" -> "TestDockerSuite|TestECS"
    """
    # Match --run "pattern" or --run pattern
    match = re.search(r'--run\s+["\']?([^"\s]+)["\']?', extra_params)
    return match.group(1) if match else None


def determine_artifacts(manifest: dict, test_pattern: str | None) -> list[str]:
    """Determine which artifacts are needed based on manifest and test pattern."""
    # Check test-specific patterns first
    if test_pattern:
        for spec in manifest.get("test_specific", []):
            pattern = spec.get("pattern", "")
            if not pattern:
                continue

            # Skip MANUAL: patterns - they can't be automatically matched
            if pattern.startswith("MANUAL:"):
                continue

            # First try exact match (for patterns with regex syntax)
            if pattern == test_pattern:
                return spec.get("artifacts", [])

            # Then try regex match (for partial matches)
            try:
                if re.search(pattern, test_pattern):
                    return spec.get("artifacts", [])
            except re.error:
                # Pattern might be invalid regex, skip
                pass

    # Fall back to default artifacts
    return manifest.get("default_artifacts", [])


def resolve_job_artifacts(job_config: dict, ci_config: dict) -> list[str]:
    """Resolve artifacts for a job, including template inheritance.

    Handles jobs that extend templates with needs sections.
    """
    artifacts = []

    # First check if job has direct needs
    if "needs" in job_config:
        artifacts.extend(extract_artifacts_from_needs(job_config["needs"], ci_config))

    # Then check templates this job extends
    extends = job_config.get("extends", [])
    if isinstance(extends, str):
        extends = [extends]

    for template_name in extends:
        if template_name in ci_config:
            template = ci_config[template_name]
            if isinstance(template, dict) and "needs" in template:
                artifacts.extend(extract_artifacts_from_needs(template["needs"], ci_config))

    return list(dict.fromkeys(artifacts))  # Remove duplicates while preserving order


def extract_artifacts_from_needs(needs: list, ci_config: dict | None = None) -> list[str]:
    """Extract artifact job names from needs section.

    Handles:
    - Simple string references: "qa_agent_linux"
    - Dict references: {"job": "qa_agent_linux", "optional": true}
    - GitLab !reference (parsed as list): [".template_name", "needs"]
    """
    artifacts = []
    artifact_prefixes = ("qa_", "agent_deb", "agent_rpm", "agent_suse", "deploy_")

    for need in needs:
        if isinstance(need, str):
            # Simple string reference
            if need.startswith(artifact_prefixes):
                artifacts.append(need)
        elif isinstance(need, dict):
            # Dictionary with optional: true or other keys
            job_name = need.get("job", "")
            if job_name.startswith(artifact_prefixes):
                artifacts.append(job_name)
        elif isinstance(need, list) and ci_config:
            # This is a !reference like [".template_name", "needs"]
            # Resolve the reference and recursively extract artifacts
            if len(need) >= 2:
                template_name = need[0]
                field_name = need[1]
                if template_name in ci_config:
                    template = ci_config[template_name]
                    if isinstance(template, dict) and field_name in template:
                        referenced_needs = template[field_name]
                        if isinstance(referenced_needs, list):
                            artifacts.extend(extract_artifacts_from_needs(referenced_needs, ci_config))

    return artifacts


def update_job_needs(job_config: dict, artifacts: list[str], ci_config: dict) -> bool:
    """Update the needs section of a job with new artifacts.

    IMPORTANT: Currently only updates jobs that have explicit needs sections.
    Jobs that inherit needs purely from templates are skipped to avoid YAML complexity.

    To update template-based jobs:
    1. Manually add a needs section with !reference to the template
    2. Or update the template itself

    Returns:
        True if the job was updated, False if skipped (template-only job)
    """
    from ruamel.yaml.comments import CommentedSeq

    # Only update jobs that already have explicit needs
    if "needs" not in job_config:
        # Skip jobs that inherit needs purely from templates
        # This avoids complex YAML reference manipulation
        return False

    # Job has explicit needs - update in place
    current_needs = job_config["needs"]
    new_needs = CommentedSeq()

    # Preserve non-artifact entries (like !reference)
    for need in current_needs:
        if isinstance(need, str):
            if not need.startswith(("qa_", "agent_deb", "agent_rpm", "agent_suse", "deploy_")):
                new_needs.append(need)
        elif isinstance(need, dict):
            # Preserve non-artifact job entries and special entries
            if "job" in need:
                job_name = need["job"]
                if not job_name.startswith(("qa_", "agent_deb", "agent_rpm", "agent_suse", "deploy_")):
                    new_needs.append(need)
            else:
                # Preserve other dict entries (like !reference)
                new_needs.append(need)
        else:
            # Preserve any other type (like tagged scalars)
            new_needs.append(need)

    # Add new artifacts
    for artifact in sorted(artifacts):
        new_needs.append(artifact)

    job_config["needs"] = new_needs
    return True
