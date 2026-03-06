# GenSim EKS Evaluator

## User Story

As a member of the observer team, I need to run gensim episodes against
custom agent builds on EKS so that (a) the observer-recorder captures
parquet data for offline testbench replay and scoring, and (b) we can
verify live anomaly detection behavior against known incident scenarios.

## Requirements

### REQ-GE-001: Submit an Evaluation Run

WHEN a developer invokes the submit command with an agent image and one or
more episode:scenario pairs
THE SYSTEM SHALL provision an EKS cluster if one does not already exist,
queue the requested episodes for serial execution, and return a run ID
immediately

WHEN the cluster already exists and is healthy
THE SYSTEM SHALL skip provisioning and queue the episodes directly

WHEN the cluster exists but is unhealthy or unreachable
THE SYSTEM SHALL report the error and refuse to queue

WHEN an orchestrator Job is already running
THE SYSTEM SHALL refuse the submission and report the active run ID

**Rationale:** Developers want fire-and-forget submission without managing
infrastructure. Making provisioning implicit and idempotent removes friction
and lets developers focus on evaluating their agent builds.

---

### REQ-GE-002: Check Evaluation Status

WHEN a developer invokes the status command
THE SYSTEM SHALL display the current state of each queued episode: queued,
running (with phase), done (with parquet count), or failed (with reason)

WHEN no evaluation is in progress
THE SYSTEM SHALL report that the cluster is idle

**Rationale:** Developers submit and walk away. They need to check progress
from any terminal without tailing logs or connecting to the cluster.

---

### REQ-GE-003: Serial Episode Execution

WHEN multiple episodes are queued
THE SYSTEM SHALL execute them one at a time in submission order

THE SYSTEM SHALL NOT run two episodes concurrently on the same cluster

**Rationale:** Concurrent episodes pollute each other's metrics, traces, and
parquet data. Serial execution ensures each dataset is attributable to
exactly one incident scenario.

---

### REQ-GE-004: Clean Isolation Between Episodes

WHEN an episode completes
THE SYSTEM SHALL uninstall the episode Helm chart and the Datadog agent
DaemonSet before starting the next episode

WHEN the agent is redeployed for the next episode
THE SYSTEM SHALL begin writing parquet files with fresh timestamps so that
each episode's data has clear temporal boundaries

**Rationale:** Without agent restart, parquet files from the previous episode
bleed into the next. Clean boundaries make it possible to attribute every
file to a specific episode without timestamp forensics.

---

### REQ-GE-005: Collect and Upload Results

WHEN an episode completes successfully
THE SYSTEM SHALL copy all parquet files from the agent pod and the result
JSON from the runner, then upload both to S3

THE SYSTEM SHALL use the path convention:
`s3://<bucket>/<image-tag>/<episode--scenario>/<gensim-sha>/<date>/`

WHEN parquet collection fails
THE SYSTEM SHALL report the failure visibly in the runner logs and continue
to the next episode

WHEN parquet collection succeeds
THE SYSTEM SHALL report the file count in the runner logs

**Rationale:** Parquet files are the primary input to the testbench for
offline replay and scoring. The result JSON captures episode timing and
monitor transitions. Both must be findable by image, episode, gensim
version, and date so the testbench can index and discover them.

---

### REQ-GE-006: Tag Runs with Version Metadata

THE SYSTEM SHALL record three version coordinates for every run:
the agent Docker image tag, the gensim-episodes git SHA, and the
episode:scenario name

THE SYSTEM SHALL use a clean checkout of gensim-episodes at a known git SHA
rather than the developer's local working directory

THE SYSTEM SHALL include all three coordinates in S3 paths, DD events, and
DD metric tags

**Rationale:** Evaluation results are meaningless without knowing which agent
build, which version of the episode scripts, and which scenario produced
them. Using a clean checkout prevents uncommitted local changes from
producing unattributable data.

---

### REQ-GE-007: Report Run Metadata to Datadog

WHEN an episode completes
THE SYSTEM SHALL emit a Datadog Event with: episode name, scenario, agent
image tag, gensim SHA, duration, outcome, and parquet file count

WHEN an episode completes
THE SYSTEM SHALL emit custom metrics: `gensim.episode.duration_seconds`,
`gensim.episode.alert_detection_seconds`, `gensim.episode.parquet_files`,
tagged by episode, scenario, agent image, and gensim SHA

**Rationale:** The team needs to track data capture quality over time
without manually inspecting S3. Dashboards and monitors on these metrics
surface regressions across agent builds and episode versions.

---

### REQ-GE-008: Cluster Lifecycle

THE SYSTEM SHALL keep the EKS cluster running between evaluation runs to
avoid provisioning delays on subsequent submissions

WHEN a developer explicitly invokes the destroy command
THE SYSTEM SHALL tear down all cluster resources

WHEN the weekly infrastructure cleanup job runs
THE SYSTEM SHALL allow the cluster to be destroyed and re-provisioned on
next submission

**Rationale:** A persistent cluster lets developers iterate quickly. Weekly
cleanup prevents resource leaks.

---

### REQ-GE-009: Weekly Automated Evaluation

WHEN the weekly cron trigger fires
THE SYSTEM SHALL submit the standard evaluation suite against the latest
observer-recorder image tag

**Rationale:** Manual weekly runs get forgotten. Automated scheduling
ensures continuous data capture for testbench replay and catches regressions
within one week.

**Dependencies:** REQ-GE-001, REQ-GE-007

---

### REQ-GE-010: Scoring Integration

WHEN the testbench scoring pipeline is ready
THE SYSTEM SHALL invoke the scoring tool after parquet upload and emit the
score as a DD metric

WHEN the observer emits anomaly detection events to Datadog
THE SYSTEM SHALL cross-reference those events against the episode's monitor
transitions to produce a detection accuracy score

**Rationale:** The capture pipeline (REQ-GE-005) is necessary but not
sufficient. The team needs automated scoring to compare agent builds
quantitatively. This requirement is deferred until the testbench scoring
integration and observer event emission are ready.

**Dependencies:** REQ-GE-005, REQ-GE-006
