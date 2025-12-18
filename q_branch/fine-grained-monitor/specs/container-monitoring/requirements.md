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
THE SYSTEM SHALL output all data to a single Parquet file with columnar storage

WHEN the monitor writes a metric
THE SYSTEM SHALL include labels identifying the container, pod, and node

WHEN the Parquet file exceeds 1 GiB in size
THE SYSTEM SHALL stop collecting new metrics and initiate graceful shutdown

WHEN the monitor shuts down
THE SYSTEM SHALL flush all buffered data and finalize the Parquet file

**Rationale:** Users analyze monitoring data after collection using tools like
DuckDB, pandas, or Spark. Parquet provides efficient columnar storage with
compression, enabling fast analytical queries. A single file simplifies data
management during development iterations. Labels enable filtering and grouping
by container or pod. The 1 GiB size limit prevents runaway disk usage during
long collection sessions or unexpectedly high metric cardinality.

---

### REQ-FM-005: Capture Delayed Metrics

WHEN a metric is recorded with a timestamp in the past (up to 60 seconds)
THE SYSTEM SHALL associate the metric with the correct time interval

WHEN the accumulator window advances
THE SYSTEM SHALL retain the previous 60 seconds of data to accommodate late
arrivals

**Rationale:** Some metrics are only available after-the-fact. A planned future
capability will intercept Agent outbound data and feed it to this monitor for
correlation. These intercepted metrics will have timestamps 15-45 seconds in the
past. The 60-second accumulator window ensures late metrics are correctly
associated with their original time interval rather than being dropped or
misattributed.

**Dependencies:** None currently. This requirement enables future integration
with Agent output interception.

---
