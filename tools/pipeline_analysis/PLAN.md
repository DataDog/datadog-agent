# Plan: Investigate the Build System, Then Build Analysis Tools

## Context

The Datadog Agent build system is a complex multi-layer system:
- ~126 GitLab CI YAML files defining 37+ stages with ~hundreds of jobs
- S3 as the primary artifact exchange between pipeline stages
- omnibus (Ruby-based) as the packaging system, with 6 projects and 31 software definitions
- `dda inv` (Python invoke) commands orchestrating Go compilation
- Bazel/bazelisk for external libraries (CPython, OpenSSL, etc.) and package layout

Goals:
1. Build understanding of the system (investigation plan)
2. Build a Python-based analysis toolchain in `tools/pipeline_analysis/`

**Constraints:**
- All new code goes in `tools/pipeline_analysis/` only
- No modifications to anything outside that directory
- Start with investigation, then build tools incrementally

---

## Part 1: Investigation Plan (How to Learn the System)

### Step 1: Map the Pipeline Stages (read: `.gitlab-ci.yml`)

Read the top-level `.gitlab-ci.yml` to get:
- The ordered list of stages (37+ stages from `.pre` to `.post`)
- The global variables that define S3 bucket names and artifact URI patterns
- The shared rule anchors (what conditions control when jobs run)
- Which files trigger which job categories (`changes:` rules)

**Key question:** What is the linear stage order, and at which stages do artifacts cross S3 boundaries?

### Step 2: Trace One Critical Path End-to-End

Trace the Linux agent package forward and backward:

Forward (producer chain):
1. `.gitlab/build/binary_build/linux.yml` — how is the agent binary compiled?
2. `.gitlab/build/package_build/linux.yml` — how is it packaged (omnibus)?
3. `.gitlab/deploy/container_build/docker_linux.yml` — how is the container built?
4. `.gitlab/deploy/trigger_distribution.yml` — how does it flow downstream?

Backward (consumer chain):
1. `.gitlab/test/e2e/e2e.yml` — what tests consume the package?
2. `.gitlab/test/install_script_testing/` — what install tests use it?

**Key question:** At each handoff, what is the exact artifact path (S3 URI or GitLab artifact path)?

### Step 3: Audit All dda inv and bazel Invocations

Grep across the repo for:
- `dda inv -- -e X.Y` in `.gitlab/**/*.yml` and `omnibus/config/software/*.rb`
- `bazelisk run ... //target:name` in `.rb` files and `.gitlab/**/*.yml`

Group by root command to find commands called from multiple places (duplicate build candidates).

**Key question:** Are there commands in both CI YAML and omnibus Ruby? Those are candidates for redundant builds.

### Step 4: Map S3 Artifact URI Patterns

From `S3_ARTIFACTS_URI` and related variables in `.gitlab-ci.yml`:
- `$S3_ARTIFACTS_URI/...` — short-lived, per-pipeline
- `$S3_PERMANENT_ARTIFACTS_URI/...` — long-lived
- `$S3_OMNIBUS_CACHE_BUCKET/...` — omnibus git cache

Grep for `$S3_CP_CMD`, `aws s3 cp`, `aws s3 sync` to find producers/consumers.

### Step 5 (Low Priority): Map the omnibus Dependency Graph

NOTE: Deferred — the omnibus dependency graph is already well understood and actively being refactored.

---

## Part 2: First Tool — GitLab CI Job Graph + SVG Visualization

### File Structure

```
tools/pipeline_analysis/
├── PLAN.md                   # This file
├── __init__.py
├── cli.py                    # Entry point: `python -m pipeline_analysis`
├── parsers/
│   ├── __init__.py
│   └── gitlab_ci.py          # YAML loader + resolver
├── graph/
│   ├── __init__.py
│   └── pipeline_graph.py     # NetworkX DAG builder
├── viz/
│   ├── __init__.py
│   └── dot_viz.py            # Graphviz DOT → SVG renderer
├── requirements.txt          # networkx, pyyaml, graphviz, click
└── tests/
    ├── test_gitlab_ci_parser.py
    └── fixtures/             # Small YAML samples for unit tests
```

### What the Parser Must Handle

GitLab CI YAML has several non-standard features:
1. `!reference [.anchor, key]` — GitLab-specific tag, not valid YAML; needs custom constructor
2. `&anchor` / `*alias` — standard YAML anchors/aliases; PyYAML handles these
3. `extends:` — job inheritance with deep-merge semantics (resolve topologically)
4. `include: [{local: path}]` — must load and merge ~126 files

### `extends:` Merge Semantics (matching GitLab behavior)
- Scalar values: child wins
- `script`, `before_script`, `after_script`: child wins (not concatenated)
- `variables`: dict — deep merge, child wins on conflict
- `rules`: child wins entirely (not merged)

### Job Data Model

```python
@dataclass
class Job:
    name: str
    stage: str
    needs: list[str]         # DAG edges (explicit ordering)
    script: list[str]        # raw script lines
    artifacts: dict          # {paths: [...], expire_in: ...}
    rules: list[dict]        # condition expressions
    trigger: dict | None     # child pipeline info
    tags: list[str]          # runner tags (arch:amd64, os:windows, etc.)
    image: str | None
```

### Graph Model

NetworkX `DiGraph`. One node per job. Edges from `needs:`.

Node attributes:
- `stage`: for stage-based layout
- `platform`: inferred from `tags` (linux/windows/mac/unknown)
- `produces_s3`: S3 URI patterns found in script
- `consumes_s3`: S3 URI patterns found in before_script/script

### Visualization Modes

1. **Stage view** (default): group jobs by stage using DOT subgraphs; show inter-stage edges
2. **Job view**: all jobs as nodes, `needs:` as edges; color by platform
3. **Artifact view**: add artifact nodes; show producer→artifact→consumer triples

Color scheme:
- Build jobs: light blue (`#cce5ff`)
- Test jobs: light green (`#ccffcc`)
- Deploy jobs: light orange (`#ffe5cc`)
- Windows jobs: yellow tint (`#ffffcc`)
- Trigger/child pipeline jobs: purple (`#e5ccff`)

### CLI Commands

```bash
# Render pipeline stage overview
python -m pipeline_analysis graph --mode stages --output pipeline.svg

# Show dependency graph for one job + its transitive ancestors
python -m pipeline_analysis graph --mode job --job datadog-agent-7-x64 --output job.svg

# List all jobs in a stage
python -m pipeline_analysis jobs --stage package_build

# Show what a job needs (direct)
python -m pipeline_analysis inputs --job datadog-agent-7-x64

# Show full transitive inputs
python -m pipeline_analysis inputs --job datadog-agent-7-x64 --transitive
```

---

## Implementation Order

1. `parsers/gitlab_ci.py` — Custom YAML loader. Test with fixture files using `!reference` and `extends:`.
2. `graph/pipeline_graph.py` — Build NetworkX DAG. Verify ~300-500 nodes, all 37 stage names present.
3. `viz/dot_viz.py` + `cli.py` — Render stage view SVG first, then job view.
4. Iterate based on what the graph reveals about gaps in understanding.
5. Omnibus/dda/bazel analysis — deferred until CI graph is working.

---

## Verification Targets

After graph is built:
- `len(G.nodes)` should be ~300-500
- All 37 stage names should appear in node attributes
- `G.predecessors('datadog-agent-7-x64')` should include `build_system-probe-x64` and `go_deps`

After SVG rendered:
- Stage view should show `binary_build` → `package_build` → `container_build` flow
- Windows jobs visually distinct from Linux jobs
- Trigger jobs to child pipelines distinguishable

---

## Key Technical Risks

1. **`!reference` tag**: GitLab CI uses it extensively. Must resolve before analysis or script content is missing.
   - Mitigation: two-pass load — first collect all anchors, then resolve references.

2. **Variable expansion in S3 URIs**: `$S3_ARTIFACTS_URI/binaries/$CI_JOB_NAME/agent` is dynamic.
   - Mitigation: normalize by replacing `$VAR_NAME` with `{VAR_NAME}` placeholders; match by pattern.

3. **Scale**: ~126 YAML files. Full merge may be slow.
   - Mitigation: cache parsed result to JSON; only re-parse when YAML files change (check mtime).

---

## Known System Facts (from initial exploration)

- Top-level `.gitlab-ci.yml` includes `.gitlab/.pre/**`, `.gitlab/build/**`, `.gitlab/deploy/**`, `.gitlab/test/**`, `.gitlab/.post/**`, `.gitlab/windows/**`
- 37 stages in order: `.pre`, `setup`, `maintenance_jobs`, `deps_build`, `deps_fetch`, `lint`, `source_test`, `source_test_stats`, `software_composition_analysis`, `binary_build`, `package_deps_build`, `kernel_matrix_testing_*`, `integration_test`, `benchmarks`, `package_build`, `packaging`, `pkg_metrics`, `container_build`, `container_scan`, `scan`, `check_deploy`, `dev_container_deploy`, `deploy_packages`, `choco_build`, `install_script_deploy`, `internal_image_deploy`, `e2e_deploy`, `install_script_testing`, `e2e_pre_test`, `e2e_init`, `e2e`, `e2e_k8s`, `e2e_install_packages`, `functional_test`, `trigger_distribution`, `dynamic_test`, `junit_upload`, `internal_kubernetes_deploy`, `post_rc_build`, `check_merge`, `.post`
- S3 buckets: `S3_ARTIFACTS_URI` (per-pipeline), `S3_PERMANENT_ARTIFACTS_URI` (long-lived), `S3_OMNIBUS_CACHE_BUCKET`, `S3_SBOM_STORAGE_URI`, `INTEGRATION_WHEELS_CACHE_BUCKET`
- Two child pipelines: `distribution.yml` (package distribution) and `smp-regression-child-pipeline.yml` (SMP regression)
- Primary artifact passing mechanism: S3 (via `$S3_CP_CMD` + `aws s3 sync`)
- Secondary: GitLab `artifacts:` (temp deps like Go module cache)
- Tertiary: `reports: dotenv:` for variable forwarding to child pipelines
