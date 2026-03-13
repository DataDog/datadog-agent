# E2E CI Dependencies Tools

Two `dda` extended commands for managing E2E test artifact dependencies:

1. **`dda e2e generate-manifests`** - Generate documentation manifests FROM CI config
2. **`dda e2e generate-ci-deps`** - Validate manifests against CI config

## CI as Source of Truth

**Important:** The GitLab CI configuration (`.gitlab/test/e2e/e2e.yml`) is the source of truth. Manifest files document what's configured in CI, they don't drive it.

## Commands

### `dda e2e generate-manifests`

**Purpose:** Generate `e2e-dependencies.yaml` manifests from CI configuration.

**Location:** `.dda/extend/commands/e2e/generate_manifests/__init__.py`

**How It Works:**
1. **Parses** `.gitlab/test/e2e/e2e.yml` to find E2E jobs
2. **Extracts** test areas from `TARGETS` variables
3. **Extracts** test patterns from `EXTRA_PARAMS --run` flags
4. **Resolves** artifacts from `needs:` (including `!reference` and template inheritance)
5. **Analyzes** artifact patterns across jobs:
   - Most common set → `default_artifacts`
   - Different sets → `test_specific` entries
   - Init jobs (`-init`, `E2E_INIT_ONLY`) → excluded
6. **Generates** manifest with comments documenting source jobs

### `dda e2e generate-ci-deps`

**Purpose:** Validate that manifests match CI configuration.

**Location:** `.dda/extend/commands/e2e/generate_ci_deps/__init__.py`

**How It Works:**
1. **Scans** for `e2e-dependencies.yaml` files in `test/new-e2e/tests/*/`
2. **Parses** `.gitlab/test/e2e/e2e.yml` to find E2E jobs
3. **Matches** jobs to manifests based on:
   - `TARGETS` variable → test area
   - `EXTRA_PARAMS --run` flag → test pattern (regex)
4. **Determines** expected artifacts from manifest
5. **Resolves** actual artifacts from CI (including `!reference` and templates)
6. **Compares** expected vs actual and reports mismatches

**Dependencies (both commands):**
- `pyyaml>=6.0` - For reading YAML files
- `ruamel.yaml>=0.17.0` - For preserving YAML formatting (generate-ci-deps only)

## Key Features

- ✅ Regex pattern matching like `--run` flag
- ✅ Template inheritance resolution (extends templates)
- ✅ Dry-run mode to preview changes
- ✅ Validation mode for CI (fail if config is out of sync)
- ✅ Preserves YAML formatting and comments
- ✅ Verbose mode for debugging

## Files Created

1. **Command implementation:**
   - `.dda/extend/commands/e2e/generate_ci_deps/__init__.py`

2. **Example manifest:**
   - `test/new-e2e/tests/containers/e2e-dependencies.yaml`

3. **Documentation:**
   - `test/new-e2e/E2E-CI-DEPENDENCIES.md`

## Quick Usage

### Generate Manifests (Typical Workflow)

```bash
# Generate manifest for a test area from CI config
dda e2e generate-manifests --area containers

# Preview what would be generated
dda e2e generate-manifests --dry-run

# Regenerate existing manifest (after CI changes)
dda e2e generate-manifests --area npm --force

# Generate for all areas
dda e2e generate-manifests
```

### Validate Manifests

```bash
# Validate all manifests match CI
dda e2e generate-ci-deps --validate

# Validate specific area
dda e2e generate-ci-deps --area containers --validate

# Show detailed validation output
dda e2e generate-ci-deps --validate --verbose
```

## Generated Manifest Format

Manifests are automatically generated and include:

```yaml
area: containers

# Most common artifact set across jobs
default_artifacts:
  - qa_agent_linux                 # Base Linux agent Docker image
  - qa_agent_linux_jmx             # Linux agent with JMX support
  - qa_dca                         # Datadog Cluster Agent
  - qa_dogstatsd                   # DogStatsD standalone binary

# Jobs with different artifact needs
test_specific:
  - pattern: "TestECSSuite"
    artifacts:
      - qa_agent
      - qa_agent_jmx
      - qa_agent_linux
      - qa_agent_linux_jmx
      - qa_dca
      - qa_dogstatsd
    comment: "From job: new-e2e-containers-ecs"
```

## Typical Workflow

1. **Modify CI** - Update `.gitlab/test/e2e/e2e.yml` with artifact dependencies
2. **Generate manifest** - `dda e2e generate-manifests --area myarea --force`
3. **Validate** - `dda e2e generate-ci-deps --validate`
4. **Commit both** - CI config and manifest together

Pre-commit hooks automatically run validation.

## Technical Details

### Pattern Extraction

Patterns are extracted from GitLab CI `EXTRA_PARAMS`:
```yaml
EXTRA_PARAMS: "--run TestKindSuite -c ddinfra:kubernetesVersion=1.27"
               ^^^^^^^^^^^^^^^^^^^^^^
               Extracted: TestKindSuite
```

### Pattern Matching (Validation)

When validating, patterns are matched in priority order:
1. **Exact match** - Pattern string equals test pattern exactly (for regex patterns with metacharacters)
2. **Regex match** - Pattern matches as regex against test pattern
3. **Default** - If no match, use `default_artifacts`

Example:
```python
# Manifest pattern: "TestEC2(VM|VMSELinux)Suite"
# Test pattern:     "TestEC2(VM|VMSELinux)Suite"
# Match type: Exact (both contain regex metacharacters)
```

### Artifact Resolution

Artifacts are resolved from CI jobs handling multiple formats:

1. **Direct needs:**
   ```yaml
   needs:
     - qa_agent_linux
     - qa_dca
   ```

2. **Dict format with options:**
   ```yaml
   needs:
     - job: qa_agent
       optional: true
   ```

3. **!reference (GitLab references):**
   ```yaml
   needs:
     - !reference [.template_name, needs]
   ```
   Recursively resolved by following the reference path.

4. **Template inheritance:**
   ```yaml
   extends: .new_e2e_template_needs_container_deploy_linux
   ```
   Inherits needs from extended templates.

### Init Job Detection

Jobs are identified as init jobs if any of:
- Job name ends with `-init`
- `E2E_INIT_ONLY: "true"` variable
- `stage: e2e_init`

Init jobs are excluded from validation (they manage their own dependencies).

### YAML Handling

- **Reading:** Custom GitLab YAML loader handles `!reference` tags
- **Writing (generate-ci-deps):** Uses `ruamel.yaml` to preserve formatting (not recommended)
- **Writing (generate-manifests):** Generates clean YAML from scratch
