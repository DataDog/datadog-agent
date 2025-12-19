# Container Monitoring

## User Story

As a developer working on the Datadog Agent, I need to capture fine-grained
resource metrics from containers running on a Kubernetes node so that I can
analyze performance characteristics and debug resource usage issues without
relying on the Agent I'm actively developing.

## Requirements

### REQ-FM-001: Discover Running Containers

WHEN the monitor starts on a Kubernetes node
THE SYSTEM SHALL discover all running containers by scanning the cgroup
filesystem

WHEN a new container starts after the monitor is running
THE SYSTEM SHALL detect and begin monitoring the new container within one
sampling interval

WHEN a container stops
THE SYSTEM SHALL stop attempting to collect metrics for that container

**Rationale:** Users need visibility into all containers without manual
configuration. In a development workflow, containers may be created and
destroyed frequently (e.g., restarting the Agent pod). Automatic discovery
eliminates the need to reconfigure the monitor for each iteration.

---

### REQ-FM-002: View Detailed Memory Usage

WHEN the monitor samples a container
THE SYSTEM SHALL capture per-process memory metrics including PSS and virtual
memory sizes

WHEN the user enables the `--verbose-perf-risk` flag
THE SYSTEM SHALL capture per-memory-region breakdown via smaps showing memory
consumption by mapped file or anonymous region

WHEN the user does not enable the `--verbose-perf-risk` flag
THE SYSTEM SHALL NOT read `/proc/<pid>/smaps` to avoid taking the kernel mm lock

WHEN the monitor samples a container
THE SYSTEM SHALL capture cgroup-level memory metrics including current usage,
limits, and memory pressure (PSI)

**Rationale:** Users investigating memory issues need multiple levels of detail.
Process-level PSS (Proportional Set Size) provides the most accurate view of
actual memory cost by accounting for shared pages proportionally. Region-level
metrics (smaps) reveal whether memory is consumed by specific libraries, heap,
or mapped files, but reading smaps acquires the kernel mm lock which can impact
the monitored process. Cgroup metrics show container-level limits and pressure.
The `--verbose-perf-risk` flag lets users opt into potentially invasive
collection when deeper analysis is needed.

---

### REQ-FM-003: View Detailed CPU Usage

WHEN the monitor samples a container
THE SYSTEM SHALL capture per-process CPU usage as both percentage and millicores

WHEN the monitor samples a container
THE SYSTEM SHALL capture cgroup-level CPU metrics including usage, throttling,
and CPU pressure (PSI)

WHEN the user configures a sampling interval
THE SYSTEM SHALL sample at the specified cadence with a default of 1 Hz

**Rationale:** Users debugging performance issues need to correlate CPU
consumption with specific processes. Millicores provide a Kubernetes-native unit
for comparison with resource requests and limits. Throttling metrics reveal when
containers hit CPU limits. Configurable sampling rate allows users to balance
data volume against resolution.

---

### REQ-FM-004: Analyze Data Post-Hoc

WHEN the monitor writes metrics
THE SYSTEM SHALL output data to Parquet files with columnar storage in a
configurable output directory

WHEN the monitor writes a metric
THE SYSTEM SHALL include labels for node_name, namespace, pod_name, pod_uid,
container_id, container_name, and qos_class when available

WHEN a rotation interval elapses (default 90 seconds)
THE SYSTEM SHALL close the current Parquet file with a valid footer and open a
new file, ensuring closed files are immediately readable

WHEN organizing output files
THE SYSTEM SHALL partition files into date and identifier subdirectories
(dt=YYYY-MM-DD/identifier=<pod-name>/) to support efficient querying and
future table format integration

WHEN naming output files
THE SYSTEM SHALL include a unique identifier (pod name or hostname) and
timestamp in the filename to prevent collisions when multiple monitor instances
write to the same directory

WHEN the total size of all Parquet files exceeds 1 GiB
THE SYSTEM SHALL stop collecting new metrics and initiate graceful shutdown

WHEN the monitor starts
THE SYSTEM SHALL write a session manifest file containing run configuration,
start time, and git revision for later context

WHEN the monitor shuts down
THE SYSTEM SHALL flush all buffered data and finalize the current Parquet file

**Rationale:** Users analyze monitoring data after collection using tools like
DuckDB, pandas, or Spark, which can query directories of Parquet files as a
single dataset. Parquet files require a footer to be readable; the 90-second
rotation interval exceeds the 60-second accumulator window, ensuring each file
contains complete time slices. Date/identifier partitioning enables efficient
queries and aligns with Iceberg/Delta/Hudi conventions for future integration.
Standardized labels provide reliable join keys for cross-container analysis.
The session manifest preserves run context for debugging sessions weeks later.
The 1 GiB total size limit prevents runaway disk usage.

---

### REQ-FM-005: Visualize Metrics Interactively

WHEN the user runs the visualization tool with a Parquet file
THE SYSTEM SHALL display a web-based interactive timeseries viewer in the
browser

WHEN viewing the timeseries
THE SYSTEM SHALL allow the user to filter by container using a dropdown or
selection control

WHEN the user selects a container
THE SYSTEM SHALL display high-resolution CPU usage over time at the full
sampling rate (1 Hz)

WHEN viewing the timeseries
THE SYSTEM SHALL enable pan/zoom interactions to explore specific time ranges
at full resolution

WHEN multiple containers are selected
THE SYSTEM SHALL overlay their timeseries on the same chart for visual
comparison of patterns

**Rationale:** Users investigating performance issues need to visually inspect
CPU usage patterns to identify oscillations, spikes, or anomalies. The automated
oscillation detector identifies candidates, but human judgment is needed to
confirm patterns and understand their characteristics. Interactive visualization
lets users quickly compare container behavior and zoom into specific time
windows. Browser-based viewing enables sharing and collaboration without
additional tooling.

---

