from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

import click
import yaml
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Generate e2e-dependencies.yaml manifests from CI config",
    dependencies=[
        "pyyaml>=6.0",
    ],
)
@click.option("--area", help="Generate only for specific test area")
@click.option("--dry-run", is_flag=True, help="Show what would be generated without writing")
@click.option("--force", is_flag=True, help="Overwrite existing manifests")
@pass_app
def cmd(app: Application, *, area: str | None, dry_run: bool, force: bool) -> None:
    """
    Generate e2e-dependencies.yaml manifests from GitLab CI configuration.

    This command reads .gitlab/test/e2e/e2e.yml and generates manifest files
    for each test area based on the current CI configuration. This makes the
    CI YAML the source of truth, and manifests document what's configured.

    Examples:

        # Generate manifests for all test areas
        dda e2e generate-manifests

        # Preview what would be generated
        dda e2e generate-manifests --dry-run

        # Generate for specific area
        dda e2e generate-manifests --area containers

        # Overwrite existing manifests
        dda e2e generate-manifests --force
    """
    repo_root = Path.cwd()
    test_root = repo_root / "test" / "new-e2e" / "tests"
    gitlab_ci_path = repo_root / ".gitlab" / "test" / "e2e" / "e2e.yml"

    if not gitlab_ci_path.exists():
        app.display_error(f"GitLab CI file not found: {gitlab_ci_path}")
        raise SystemExit(1)

    app.display("🔍 Reading GitLab CI configuration...")

    # Parse GitLab CI YAML - use safe_load with custom constructors for GitLab tags
    # We need to handle !reference tags that GitLab CI uses
    class GitLabLoader(yaml.SafeLoader):
        pass

    def reference_constructor(loader, node):
        # Represent !reference as a simple list for our purposes
        return loader.construct_sequence(node)

    GitLabLoader.add_constructor('!reference', reference_constructor)

    with open(gitlab_ci_path) as f:
        ci_config = yaml.load(f, Loader=GitLabLoader)

    if ci_config is None:
        app.display_error("Failed to parse GitLab CI YAML")
        raise SystemExit(1)

    # Collect jobs by test area with their artifacts and test patterns
    area_jobs = {}  # area -> list of (test_pattern, artifacts)

    for job_name, job_config in ci_config.items():
        if not isinstance(job_config, dict):
            continue

        # Only process E2E test jobs
        if not job_name.startswith("new-e2e-"):
            continue

        # Resolve variables (including from extended templates)
        variables = resolve_job_variables(job_config, ci_config)
        targets = variables.get("TARGETS", "")
        extra_params = variables.get("EXTRA_PARAMS", "")

        if not targets:
            continue

        # Extract test area from TARGETS
        test_area = extract_test_area(targets)
        if not test_area:
            continue

        # Filter by area if specified
        if area and test_area != area:
            continue

        # Get artifacts for this job
        artifacts = resolve_job_artifacts(job_config, ci_config)

        # Extract test pattern from EXTRA_PARAMS --run flag
        test_pattern = extract_test_pattern(extra_params)

        # Check if this is an init job
        is_init = (
            job_name.endswith("-init")
            or variables.get("E2E_INIT_ONLY") == "true"
            or job_config.get("stage") == "e2e_init"
        )

        if test_area not in area_jobs:
            area_jobs[test_area] = []

        area_jobs[test_area].append(
            {
                'job_name': job_name,
                'pattern': test_pattern,
                'artifacts': artifacts,
                'is_init': is_init,
            }
        )

    if not area_jobs:
        if area:
            app.display_warning(f"No jobs found for area: {area}")
        else:
            app.display_info("No E2E jobs with artifacts found")
        return

    app.display(f"\n📋 Found {len(area_jobs)} test area(s) with jobs:")
    for test_area in sorted(area_jobs.keys()):
        app.display(f"  • {test_area}: {len(area_jobs[test_area])} job(s)")

    # Generate manifests
    generated = 0
    skipped = 0

    for test_area, jobs in sorted(area_jobs.items()):
        manifest_path = test_root / test_area / "e2e-dependencies.yaml"

        # Check if manifest already exists
        if manifest_path.exists() and not force and not dry_run:
            skipped += 1
            if not area:  # Only show in full scan
                app.display(f"  ⏭️  Skipped {test_area} (manifest exists, use --force to overwrite)")
            continue

        # Analyze jobs to determine default artifacts and test-specific overrides
        manifest_data = analyze_area_jobs(test_area, jobs)

        # Generate manifest content
        manifest_content = generate_manifest_from_data(test_area, manifest_data)

        if dry_run:
            app.display(f"\n📝 Would create: {manifest_path.relative_to(repo_root)}")
            app.display(f"   Default artifacts: {len(manifest_data['default_artifacts'])}")
            if manifest_data['test_specific']:
                app.display(f"   Test-specific patterns: {len(manifest_data['test_specific'])}")
        else:
            # Write manifest
            manifest_path.parent.mkdir(parents=True, exist_ok=True)
            with open(manifest_path, "w") as f:
                f.write(manifest_content)
            generated += 1
            rel_path = manifest_path.relative_to(repo_root)
            app.display(f"  ✅ Created: {rel_path}")

    # Summary
    app.display("")
    if dry_run:
        app.display(f"✨ Would create {len(area_jobs)} manifest(s)")
        if skipped > 0:
            app.display(f"   {skipped} already exist (use --force to overwrite)")
    else:
        if generated > 0:
            app.display_success(f"✅ Created {generated} manifest(s)")
        if skipped > 0:
            app.display_info(f"   {skipped} skipped (already exist)")
        if generated > 0:
            app.display("\n💡 Next: Run 'dda e2e generate-ci-deps --validate' to verify")


def resolve_job_variables(job_config: dict, ci_config: dict) -> dict:
    """Resolve variables for a job, including template inheritance.

    Variables from extended templates are merged, with job-specific variables taking precedence.
    """
    merged_vars = {}

    # First collect variables from extended templates (in order)
    extends = job_config.get("extends", [])
    if isinstance(extends, str):
        extends = [extends]

    for template_name in extends:
        if template_name in ci_config:
            template = ci_config[template_name]
            if isinstance(template, dict) and "variables" in template:
                merged_vars.update(template["variables"])

    # Then overlay job-specific variables (these take precedence)
    if "variables" in job_config:
        merged_vars.update(job_config["variables"])

    return merged_vars


def extract_test_area(targets: str) -> str | None:
    """Extract test area from TARGETS variable."""
    import re

    match = re.search(r"\.?/tests/([^/\s]+)", targets)
    return match.group(1) if match else None


def extract_test_pattern(extra_params: str) -> str | None:
    """Extract test pattern from EXTRA_PARAMS --run flag.

    Examples:
        "--run TestKindSuite" -> "TestKindSuite"
        "--run 'TestDockerSuite|TestECS'" -> "TestDockerSuite|TestECS"
    """
    import re

    # Match --run "pattern" or --run pattern
    match = re.search(r'--run\s+["\']?([^"\s]+)["\']?', extra_params)
    return match.group(1) if match else None


def analyze_area_jobs(area: str, jobs: list[dict]) -> dict:
    """Analyze jobs in an area to determine default artifacts and test-specific overrides.

    Strategy:
    1. Find the most common artifact set (used by jobs without specific test patterns)
    2. Jobs with test patterns that differ from default become test-specific overrides
    3. Init jobs with no artifacts only get entries if there's no non-init job with same pattern
    """
    from collections import Counter

    # First, find all patterns used by non-init jobs
    non_init_patterns = {job['pattern'] for job in jobs if job['pattern'] and not job['is_init']}

    # Separate jobs by whether they have test patterns and artifacts
    jobs_with_pattern = []
    jobs_without_pattern = []
    standalone_init_jobs = []  # Init jobs with no corresponding non-init job

    for job in jobs:
        # Skip init jobs - they'll be handled separately
        if job['is_init']:
            # Only track init jobs that don't have a corresponding non-init job
            if job['pattern'] and job['pattern'] not in non_init_patterns:
                standalone_init_jobs.append(job)
            continue

        if not job['artifacts']:
            # Non-init job with no artifacts (shouldn't happen often)
            continue

        if job['pattern']:
            jobs_with_pattern.append(job)
        else:
            jobs_without_pattern.append(job)

    # Find the most common artifact set among jobs without patterns
    # This becomes our default
    artifact_sets = []
    for job in jobs_without_pattern:
        artifact_sets.append(tuple(sorted(job['artifacts'])))

    if artifact_sets:
        # Most common artifact set
        counter = Counter(artifact_sets)
        default_artifacts = list(counter.most_common(1)[0][0])
    else:
        # No jobs without patterns, use most common from all jobs
        for job in jobs_with_pattern:
            artifact_sets.append(tuple(sorted(job['artifacts'])))
        if artifact_sets:
            counter = Counter(artifact_sets)
            default_artifacts = list(counter.most_common(1)[0][0])
        else:
            default_artifacts = []

    # Find test-specific overrides
    test_specific = []

    # Add standalone init jobs with empty artifacts first (only if no non-init job with same pattern)
    for job in standalone_init_jobs:
        test_specific.append(
            {
                'pattern': job['pattern'],
                'artifacts': [],
                'comment': f"Init job (no artifacts needed): {job['job_name']}",
            }
        )

    # Add jobs WITHOUT patterns that have different artifacts from default
    # These need synthetic patterns based on job names since they don't have --run patterns
    for job in jobs_without_pattern:
        job_artifacts = sorted(job['artifacts'])
        if job_artifacts != sorted(default_artifacts):
            # Extract a distinguishing feature from the job name for synthetic pattern
            # e.g., "new-e2e-package-signing-debian" -> "debian"
            # This creates a comment that helps identify which job this is for
            synthetic_comment = f"No test pattern - from job: {job['job_name']}"

            # Create entry noting this job has different artifacts but no pattern
            # We use a comment-only entry since we can't create a matching pattern
            test_specific.append(
                {
                    'pattern': f"MANUAL:{job['job_name']}",  # Prefix with MANUAL: to indicate manual handling needed
                    'artifacts': job_artifacts,
                    'comment': f"{synthetic_comment} (requires manual pattern or CI update)",
                }
            )

    # Add jobs WITH patterns that have different artifacts
    for job in jobs_with_pattern:
        job_artifacts = sorted(job['artifacts'])
        if job_artifacts != sorted(default_artifacts):
            # This job needs different artifacts
            test_specific.append(
                {
                    'pattern': job['pattern'],
                    'artifacts': job_artifacts,
                    'comment': f"From job: {job['job_name']}",
                }
            )

    return {
        'default_artifacts': default_artifacts,
        'test_specific': test_specific,
    }


def generate_manifest_from_data(area: str, manifest_data: dict) -> str:
    """Generate manifest file content from analyzed data."""
    content = f"""# E2E CI Artifact Dependencies for {area} tests
# This file defines which GitLab CI build artifacts (qa_* jobs) are needed
# for E2E tests in this area.
#
# Generated from: .gitlab/test/e2e/e2e.yml
# Validate: dda e2e generate-ci-deps --validate
# Regenerate: dda e2e generate-manifests --area {area} --force

area: {area}

# Default artifacts used by tests in this area
# Generated from current CI configuration
default_artifacts:
"""

    for artifact in manifest_data['default_artifacts']:
        # Add comment based on artifact type
        comment = get_artifact_comment(artifact)
        content += f"  - {artifact:<30} # {comment}\n"

    if manifest_data['test_specific']:
        content += "\n# Test-specific overrides\n"
        content += "# These patterns match specific tests that need different artifacts\n"
        content += "test_specific:\n"
        for spec in manifest_data['test_specific']:
            content += f"  - pattern: \"{spec['pattern']}\"\n"
            if spec['artifacts']:
                content += "    artifacts:\n"
                for artifact in spec['artifacts']:
                    content += f"      - {artifact}\n"
            else:
                # Empty artifact list (init jobs)
                content += "    artifacts: []\n"
            content += f"    comment: \"{spec['comment']}\"\n"
    else:
        content += """
# Test-specific overrides (optional)
# Add patterns here to specify different artifacts for specific test patterns
# Example:
# test_specific:
#   - pattern: "TestDockerSuite"
#     artifacts:
#       - qa_agent_linux
#       - qa_dogstatsd
#     comment: "Docker tests only need these artifacts"
"""

    return content


def resolve_job_artifacts(job_config: dict, ci_config: dict) -> list[str]:
    """Resolve artifacts for a job, including template inheritance."""
    artifacts = []

    # Check if job has direct needs
    if "needs" in job_config:
        artifacts.extend(extract_artifacts_from_needs(job_config["needs"], ci_config))

    # Check templates this job extends
    extends = job_config.get("extends", [])
    if isinstance(extends, str):
        extends = [extends]

    for template_name in extends:
        if template_name in ci_config:
            template = ci_config[template_name]
            if isinstance(template, dict) and "needs" in template:
                artifacts.extend(extract_artifacts_from_needs(template["needs"], ci_config))

    return list(dict.fromkeys(artifacts))  # Remove duplicates, preserve order


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
            if need.startswith(artifact_prefixes):
                artifacts.append(need)
        elif isinstance(need, dict):
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


def get_artifact_comment(artifact: str) -> str:
    """Get a descriptive comment for an artifact."""
    if artifact.startswith("qa_agent_linux"):
        if "jmx" in artifact:
            return "Linux agent with JMX support"
        elif "fips" in artifact:
            return "FIPS-compliant Linux agent"
        else:
            return "Base Linux agent Docker image"
    elif artifact.startswith("qa_dca"):
        if "fips" in artifact:
            return "FIPS-compliant cluster agent"
        else:
            return "Datadog Cluster Agent"
    elif artifact.startswith("qa_dogstatsd"):
        return "DogStatsD standalone binary"
    elif artifact.startswith("qa_agent"):
        if "jmx" in artifact:
            return "Windows agent with JMX"
        else:
            return "Windows agent"
    elif artifact.startswith("agent_deb"):
        if "fips" in artifact:
            return "FIPS Debian package"
        else:
            return "Debian package"
    elif artifact.startswith("agent_rpm"):
        return "RPM package"
    elif artifact.startswith("deploy_deb"):
        return "Debian deployment package"
    elif artifact.startswith("deploy_rpm"):
        return "RPM deployment package"
    elif artifact.startswith("deploy_suse"):
        return "SUSE RPM deployment package"
    elif artifact.startswith("deploy_installer"):
        return "Installer OCI artifact"
    elif artifact.startswith("deploy_agent_oci"):
        return "Agent OCI artifact"
    elif artifact.startswith("qa_cws"):
        return "CWS instrumentation artifact"
    else:
        return "Artifact"
