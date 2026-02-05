# Critical Path Analysis for GitLab Pipelines

This module provides tools to analyze critical paths in GitLab CI pipelines using data from Datadog CI Visibility.

## Overview

The critical path analyzer:
1. Fetches pipeline and job data from Datadog CI Visibility API
2. Constructs dependency graphs (DAGs) from job timing and GitLab CI config
3. Computes the longest duration path (critical path) through each pipeline
4. Aggregates results across multiple pipelines to identify bottlenecks

## Prerequisites

### Environment Variables

The tool requires Datadog API credentials to be set:

```bash
export DD_API_KEY="your_api_key"
export DD_APP_KEY="your_app_key"
export DD_SITE="datadoghq.com"  # or your Datadog site
```

In CI environments, these are typically set automatically via secrets management.

### Local Setup

To run locally, you need to:
1. Obtain Datadog API credentials with CI Visibility access
2. Set the environment variables above
3. Ensure you have network access to the Datadog API

## Usage

### Basic Usage

Analyze the last 100 pipelines on the main branch:

```bash
dda inv pipeline.analyze-critical-path
```

### Custom Parameters

```bash
# Analyze last 50 pipelines
dda inv pipeline.analyze-critical-path --limit=50

# Analyze a specific branch
dda inv pipeline.analyze-critical-path --branch=7.64.x

# Filter by pipeline status
dda inv pipeline.analyze-critical-path --status-filter=success

# Change output directory
dda inv pipeline.analyze-critical-path --output-dir=./my-analysis

# Generate only JSON output
dda inv pipeline.analyze-critical-path --output-format=json

# Verbose mode for debugging
dda inv pipeline.analyze-critical-path --limit=10 --verbose
```

### Output Formats

The tool can generate three types of reports:

1. **JSON Report** (`critical-path-report.json`):
   - Complete data in machine-readable format
   - Includes all statistics, path patterns, and examples

2. **CSV Reports** (3 files):
   - `path_patterns.csv`: Most common critical path patterns
   - `job_frequency.csv`: Jobs that appear most in critical paths
   - `job_duration_stats.csv`: Duration statistics per job

3. **Text Report** (`critical-path-report.txt`):
   - Human-readable summary
   - Top 10 path patterns
   - Top 20 most frequent jobs
   - Duration statistics table

## Architecture

### Modules

- **critical_path.py**: Core data structures (JobNode, PipelineDAG, CriticalPath, CriticalPathStats)
- **critical_path_data.py**: Data fetching from Datadog CI Visibility API
- **critical_path_gitlab.py**: GitLab CI config parsing and dependency extraction
- **critical_path_algo.py**: Critical path algorithm implementation (topological sort + longest path)
- **critical_path_report.py**: Report generation (JSON/CSV/text)

### Algorithm

The critical path is computed using the **longest path in a DAG** algorithm:

1. **Topological Sort**: Order jobs so dependencies come before dependents
2. **Dynamic Programming**: Compute longest path to each node
   - `dist[v] = max(dist[predecessor] + duration[predecessor])` for all predecessors
3. **Backtracking**: Reconstruct path from the node with maximum distance

Time complexity: O(V + E) where V = number of jobs, E = number of dependencies

### Data Flow

```
Datadog API (pipelines/jobs) ──┐
                               ├──> Build DAG ──> Compute Critical Path ──> Aggregate ──> Generate Reports
GitLab CI Config (dependencies) ┘
```

## Example Output

```
================================================================================
Critical Path Analysis Report
================================================================================

Analyzed 100 pipelines
  - Successful: 87
  - Failed: 13
  - Average critical path duration: 39.0 minutes

--------------------------------------------------------------------------------
Top 10 Most Common Critical Path Patterns
--------------------------------------------------------------------------------
1. [72/100 = 72.0%] go_deps -> binary_build -> package_build -> e2e_tests
2. [15/100 = 15.0%] go_deps -> binary_build -> integration_tests
...

--------------------------------------------------------------------------------
Top 20 Jobs Appearing in Critical Paths
--------------------------------------------------------------------------------
Rank  Job Name                                           Frequency   Avg Duration
--------------------------------------------------------------------------------
1     go_deps                                            98/100 (98%)   2.2 min
2     binary_build                                       95/100 (95%)  12.5 min
...
```

## Troubleshooting

### 401 Unauthorized Error

```
Error fetching pipelines: (401) Reason: Unauthorized
```

**Solution**: Ensure `DD_API_KEY` and `DD_APP_KEY` environment variables are set with valid credentials.

### No Pipelines Found

```
No pipelines found matching the criteria
```

**Possible causes**:
- Branch name is incorrect
- No pipelines in the last 30 days (default lookback)
- Project name mismatch (hardcoded to "DataDog/datadog-agent")

### Temporal Inconsistency Warnings

```
Warning: Temporal inconsistency detected: job1 ended at ... but job2 started at ...
```

This indicates that the dependency graph from GitLab config doesn't match the actual job execution times. This can happen if:
- Jobs are misconfigured in GitLab CI
- There are race conditions in job execution
- The data from Datadog API is incomplete

These warnings don't affect the critical path computation but may indicate pipeline issues.

## Future Enhancements

Potential improvements:
- Visualization of critical paths (Graphviz, mermaid)
- Historical trend analysis
- Optimization suggestions (which jobs to parallelize)
- Stage-level analysis
- Comparison between branches or time periods
