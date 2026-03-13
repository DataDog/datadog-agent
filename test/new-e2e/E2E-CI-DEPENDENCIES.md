# E2E CI Artifact Dependencies

This document explains how to manage GitLab CI artifact dependencies for E2E tests using the `e2e-dependencies.yaml` manifest files.

## Overview

E2E tests run in GitLab CI and depend on various build artifacts (agent Docker images, binaries, etc.). These dependencies are defined in `.gitlab/test/e2e/e2e.yml` using templates and direct `needs:` sections.

**The CI configuration (`.gitlab/test/e2e/e2e.yml`) is the source of truth.** Manifest files (`e2e-dependencies.yaml`) document what artifacts are needed and can be generated from the CI configuration.

## Quick Start

### 1. Generate Manifest from CI

Generate a manifest that documents your test area's current CI configuration:

```bash
# Preview what would be generated
dda e2e generate-manifests --area myarea --dry-run

# Generate the manifest
dda e2e generate-manifests --area myarea

# Generate manifests for all areas
dda e2e generate-manifests
```

The generated manifest will look like:
```yaml
# test/new-e2e/tests/myarea/e2e-dependencies.yaml
area: myarea

default_artifacts:
  - qa_agent_linux

test_specific:
  - pattern: "TestK8sSuite"
    artifacts:
      - qa_agent_linux
      - qa_dca
    comment: "From job: new-e2e-myarea-k8s"
```

### 2. Validate Manifest Against CI

Ensure your manifest matches the CI configuration:

```bash
# Validate all manifests
dda e2e generate-ci-deps --validate

# Validate specific area
dda e2e generate-ci-deps --area myarea --validate
```

### 3. Add Validation to Pre-commit

Validation is automatically run by the pre-commit hook:

```yaml
# .pre-commit-config.yaml
- id: e2e-ci-dependencies
  entry: 'dda e2e generate-ci-deps --validate'
  files: (test/new-e2e/tests/.*/e2e-dependencies\.yaml|\.gitlab/test/e2e/e2e\.yml)$
```

## Manifest File Format

### Full Example

```yaml
# test/new-e2e/tests/containers/e2e-dependencies.yaml
area: containers

# Default artifacts used by all tests unless overridden
default_artifacts:
  - qa_agent_linux       # Base Linux agent
  - qa_agent_linux_jmx   # Linux agent with JMX
  - qa_dca               # Cluster agent
  - qa_dogstatsd         # DogStatsD binary

# Test-specific overrides (regex patterns like --run flag)
test_specific:
  - pattern: "TestKindSuite|TestEKSSuite"
    artifacts:
      - qa_agent_linux
      - qa_dca
    comment: "K8s tests only need base + cluster agent"

  - pattern: "TestDockerSuite"
    artifacts:
      - qa_agent_linux
      - qa_dogstatsd
    comment: "Docker tests need dogstatsd"
```

### Fields

- **`area`** (required): Test area name (matches directory name)
- **`default_artifacts`** (required): Artifacts used by all tests in this area unless overridden
- **`test_specific`** (optional): List of pattern-based overrides
  - **`pattern`**: Regex matching test names (like `--run` flag in CI)
  - **`artifacts`**: Artifacts needed for tests matching this pattern
  - **`comment`**: Human-readable explanation (optional)

### Available Artifacts

Common artifact jobs:
- `qa_agent_linux` - Linux agent Docker image
- `qa_agent_linux_jmx` - Linux agent with JMX support
- `qa_dca` - Datadog Cluster Agent
- `qa_dogstatsd` - DogStatsD standalone binary
- `qa_agent` - Windows agent (optional)
- `qa_agent_jmx` - Windows agent with JMX (optional)
- `agent_deb-x64-a7` - Debian package
- `agent_deb-x64-a7-fips` - FIPS Debian package

## Command Reference

### `dda e2e generate-manifests`

Generate manifest files from GitLab CI configuration.

**Options:**
- `--area TEXT` - Generate only for specific test area
- `--dry-run` - Preview what would be generated without writing
- `--force` - Overwrite existing manifests

**Examples:**

```bash
# Generate manifests for all test areas
dda e2e generate-manifests

# Preview what would be generated
dda e2e generate-manifests --dry-run

# Generate for specific area
dda e2e generate-manifests --area containers

# Regenerate existing manifest
dda e2e generate-manifests --area containers --force
```

### `dda e2e generate-ci-deps`

Validate that manifests match GitLab CI configuration.

**Options:**
- `--area TEXT` - Validate only specific test area
- `--validate` - Validation mode (exits with error if mismatches found)
- `--dry-run` - Preview what would change (not recommended for normal use)
- `--verbose` - Show detailed output

**Examples:**

```bash
# Validate all manifests (recommended)
dda e2e generate-ci-deps --validate

# Validate specific area
dda e2e generate-ci-deps --area containers --validate

# Show detailed validation info
dda e2e generate-ci-deps --validate --verbose
```

**Note:** The `generate-ci-deps` command without `--validate` can modify CI configuration, but this is **not recommended** as it may break YAML formatting. Always keep CI as the source of truth and use `generate-manifests` to update documentation.

## How It Works

### Manifest Generation (`generate-manifests`)

The tool analyzes the CI configuration and generates manifests:

1. **Scans** `.gitlab/test/e2e/e2e.yml` for E2E jobs
2. **Extracts** test area from `TARGETS` (e.g., `./tests/containers` → `containers`)
3. **Extracts** test patterns from `EXTRA_PARAMS --run` flag (e.g., `--run TestKindSuite`)
4. **Resolves** artifacts from `needs:` sections (including `!reference` and template inheritance)
5. **Analyzes** which jobs use which artifacts:
   - Most common artifact set becomes `default_artifacts`
   - Jobs with different artifacts get `test_specific` entries
   - Init jobs (ending in `-init` or `E2E_INIT_ONLY`) are automatically excluded
6. **Generates** manifest file with comments documenting the source

Example CI job:
```yaml
new-e2e-containers-kind:
  extends: .new_e2e_template_needs_container_deploy_linux
  variables:
    TARGETS: ./tests/containers
    EXTRA_PARAMS: "--run TestKindSuite"
```

Becomes manifest entry:
```yaml
area: containers
default_artifacts:
  - qa_agent_linux
  - qa_dca
```

### Validation (`generate-ci-deps --validate`)

The validation tool ensures manifests match CI:

1. **Loads** all `e2e-dependencies.yaml` manifests
2. **Scans** CI jobs and extracts their actual artifact needs
3. **Determines** expected artifacts based on manifest (matching test patterns)
4. **Compares** expected vs actual artifacts
5. **Reports** mismatches with commands to fix them

### Special Cases Handled

- **!reference resolution**: GitLab `!reference` tags are recursively resolved
- **Template inheritance**: Jobs extending templates inherit their needs
- **Init jobs**: Jobs with `-init` suffix, `E2E_INIT_ONLY`, or `e2e_init` stage are skipped
- **Regex patterns**: Patterns are matched exactly first, then as regex
- **Optional artifacts**: Jobs with `optional: true` in needs are included

### Priority Order

When validating artifacts for a test:
1. Exact pattern match in `test_specific` wins
2. Regex pattern match in `test_specific` (first match wins)
3. If no match, use `default_artifacts`

## CI Integration

### Pre-commit Hook

Validation is automatically enforced via pre-commit hooks (already configured in `.pre-commit-config.yaml`):

```yaml
- id: e2e-ci-dependencies
  entry: 'dda e2e generate-ci-deps --validate'
  files: (test/new-e2e/tests/.*/e2e-dependencies\.yaml|\.gitlab/test/e2e/e2e\.yml)$
```

This runs whenever you modify:
- Any `e2e-dependencies.yaml` file
- `.gitlab/test/e2e/e2e.yml`

### CI Pipeline (Optional)

Add validation to your CI pipeline:

```yaml
validate-e2e-deps:
  stage: lint
  script:
    - dda e2e generate-ci-deps --validate
  only:
    changes:
      - test/new-e2e/tests/**/e2e-dependencies.yaml
      - .gitlab/test/e2e/e2e.yml
```

## Workflow

### Typical Development Flow

1. **Modify CI config** (`.gitlab/test/e2e/e2e.yml`) - Add/change artifact dependencies
2. **Regenerate manifest** - `dda e2e generate-manifests --area myarea --force`
3. **Validate** - `dda e2e generate-ci-deps --validate` (or let pre-commit do it)
4. **Commit both files** - CI config and updated manifest

### Adding a New Test Area

1. **Add E2E jobs** to `.gitlab/test/e2e/e2e.yml` with appropriate `needs:`
2. **Generate manifest** - `dda e2e generate-manifests --area newarea`
3. **Validate** - `dda e2e generate-ci-deps --area newarea --validate`
4. **Commit** both CI config and manifest

### Updating Dependencies

**✅ Recommended approach (CI as source of truth):**
1. Update `.gitlab/test/e2e/e2e.yml` directly
2. Run `dda e2e generate-manifests --area myarea --force`
3. Commit both files

**❌ Not recommended (manifest-driven):**
1. Update `e2e-dependencies.yaml`
2. Run `dda e2e generate-ci-deps --area myarea` (may break YAML formatting)
3. Fix any YAML issues manually

**Why CI is source of truth:** The CI config has the final say on what runs. Manifests are documentation that should reflect CI, not drive it.

## Best Practices

### 1. CI is Source of Truth
Always modify `.gitlab/test/e2e/e2e.yml` first, then regenerate manifests. Don't edit manifests manually.

```bash
# ✅ Good workflow
vim .gitlab/test/e2e/e2e.yml  # Edit CI config
dda e2e generate-manifests --area myarea --force  # Update docs

# ❌ Bad workflow
vim test/new-e2e/tests/myarea/e2e-dependencies.yaml  # Edit manifest
dda e2e generate-ci-deps --area myarea  # Try to update CI (may break)
```

### 2. Regenerate After CI Changes
Whenever you change artifact dependencies in CI, regenerate the affected manifests:

```bash
dda e2e generate-manifests --area myarea --force
```

### 3. Use Validation Early
Run validation before committing:

```bash
dda e2e generate-ci-deps --validate
```

The pre-commit hook will catch issues, but it's faster to check manually first.

### 4. Keep Manifests in Sync
Manifests document the current CI behavior. If validation fails, either:
- Regenerate manifest: `dda e2e generate-manifests --area myarea --force`
- Or fix the CI config if it's wrong

### 5. Don't Manually Edit Generated Manifests
Generated manifests have comments like "From job: new-e2e-myarea-test". These are automatically created. Manual edits will be lost on regeneration.

## Troubleshooting

### "No e2e-dependencies.yaml found"
Generate the manifest from CI config:
```bash
dda e2e generate-manifests --area myarea
```

### "Validation failed: Found X mismatch(es)"
The manifest doesn't match CI. Regenerate it:
```bash
dda e2e generate-manifests --area <area> --force
```

Then validate:
```bash
dda e2e generate-ci-deps --area <area> --validate
```

### Manifest Shows Wrong Artifacts
The manifest reflects what's in CI. If it's wrong:
1. Check `.gitlab/test/e2e/e2e.yml` for the actual job configuration
2. Update the CI config
3. Regenerate the manifest

### Pattern Not Matching in Validation
If validation says expected artifacts don't match actual:
1. Check the test pattern extracted from `EXTRA_PARAMS --run`
2. Verify the manifest has a matching pattern in `test_specific`
3. Run with `--verbose` to see detailed matching info

### Pre-commit Hook Failing
```bash
# See what's wrong
dda e2e generate-ci-deps --validate

# Fix by regenerating
dda e2e generate-manifests --area <area> --force
```

## Examples

See existing manifests:
- `test/new-e2e/tests/containers/e2e-dependencies.yaml`

## Future Enhancements

Potential improvements:
- Support for conditional artifacts based on OS/platform
- Artifact dependency graphs
- Automatic detection of unused artifacts
- Integration with artifact build times for optimization
