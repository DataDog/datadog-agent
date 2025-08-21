# Dynamic Test Index Utilities

This package provides utilities to compute indexes that determine which tests should be executed based on code changes, enabling intelligent test selection in CI/CD pipelines.

## Overview

The dynamic test system enables **selective test execution** by:
1. **Indexing** - Building reverse indexes that map code packages to tests that exercise them
2. **Executing** - Determining which tests to run AND skip for a given set of changes
3. **Evaluating** - Measuring index effectiveness by comparing predicted vs actual test execution

This approach can significantly reduce CI execution time and costs while maintaining test coverage quality.

## Core Components

### Index Model (`index.py`)

The `DynamicTestIndex` class stores reverse mappings from code packages to tests:

```json
{
  "job-name": {
    "package-name": ["test1", "test2", ...]
  }
}
```

**Core functionality:**
- **Impact analysis**: `impacted_tests()` - Tests that must run due to changes
- **Skip optimization**: `skipped_tests()` - Tests that can be safely skipped  
- **Multi-job support**: `*_per_job()` variants for all jobs at once
- **Index management**: `add_tests()`, `merge()` for building and combining indexes

### Backend Storage (`backend.py`)

Handles persistent storage and retrieval of indexes:

- **`DynTestBackend`** - Abstract interface for pluggable storage backends
- **`S3Backend`** - Production S3 implementation with organized layout:
  ```
  s3://bucket/dynamic_test/<index_kind>/<commit_sha>/<job_id>/index.json
  ```

**Features:** Upload/download indexes, list available commits, consolidate multi-job indexes

### Test Execution (`executor.py`)

**`DynTestExecutor`** bridges stored indexes and actual test execution with lazy loading:

**Workflow:**
1. Initialize with backend, index kind, and commit SHA (no index loaded yet)
2. Index is automatically loaded when first accessed
3. Provides methods to determine which tests to run for given code changes

**Key capabilities:**
- **Lazy Loading**: Defers expensive index loading until actually needed
- **Error Handling**: Allows evaluator to handle index loading failures gracefully
- **Ancestor Discovery**: Finds the closest available index when exact commit isn't indexed

### Index Generation (`indexer.py`, `indexers/`)

Creates indexes from various data sources:

- **`DynTestIndexer`** - Abstract interface supporting multiple indexing strategies
- **`CoverageDynTestIndexer`** - Go coverage-based implementation

**Coverage indexing process:**
1. Scans test suite directories containing coverage data and metadata
2. Converts Go coverage data to text format using `go tool covdata`
3. Parses coverage to identify which packages each test exercises
4. Builds reverse index mapping packages → tests

### Evaluation (`evaluator.py`)

Measures and monitors index effectiveness with error reporting:

- **`DynTestEvaluator`** - Abstract interface for evaluation against CI systems
- **`DatadogDynTestEvaluator`** - Datadog CI Visibility integration

**Key features:**
- **Error Monitoring**: Automatically sends error events to Datadog when index loading fails
- **Graceful Initialization**: `initialize()` method handles index loading with error reporting
- **Analysis**: Comparison of predicted vs actual test execution
- **Miss Detection**: Identifies failing tests that would be skipped
- **Metrics**: Git-diff style output and performance monitoring over time

## Index Types

Currently supports:
- `PACKAGE` - Maps Go packages to tests that exercise them

## Complete Workflows

### 1. Building and Storing an Index
```python
from tasks.libs.dynamic_test.indexers.e2e import CoverageDynTestIndexer
from tasks.libs.dynamic_test.backend import S3Backend
from tasks.libs.dynamic_test.index import IndexKind

# Generate index from coverage data
indexer = CoverageDynTestIndexer("/path/to/coverage")
index = indexer.compute_index(ctx)

# Store in S3 for later use
backend = S3Backend("s3://my-bucket/ci-artifacts")
backend.upload_index(index, IndexKind.PACKAGE, commit_sha)
```

### 2. Smart Test Execution
```python
from tasks.libs.dynamic_test.executor import DynTestExecutor

# Create executor with lazy index loading
executor = DynTestExecutor(ctx, backend, IndexKind.PACKAGE, current_commit)

# Determine test execution strategy (index loaded automatically)
changed_packages = ["pkg/collector", "pkg/api"] 
tests_to_run = executor.tests_to_run("unit-tests", changed_packages)
tests_to_skip = executor.index().skipped_tests(changed_packages, "unit-tests")

print(f"Running {len(tests_to_run)} tests, skipping {len(tests_to_skip)}")
# Execute only the necessary tests in your CI system
```

### 3. Monitoring Index Quality
```python
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator

# Create evaluator with executor (index not loaded yet)
evaluator = DatadogDynTestEvaluator(ctx, IndexKind.PACKAGE, executor, pipeline_id)

# Initialize index with error reporting to Datadog
if evaluator.initialize():
    # Evaluate how well the index predicted test execution
    results = evaluator.evaluate(changed_packages)
    evaluator.print_summary(results)
    evaluator.send_stats_to_datadog(results)
else:
    print("Index initialization failed - error sent to Datadog")
```

## File Structure

```
tasks/libs/dynamic_test/
├── backend.py           # Storage backends (S3)
├── evaluator.py         # Index effectiveness evaluation
├── executor.py          # Test execution logic
├── index.py             # Core index model
├── indexer.py           # Index generation interface
└── indexers/
    └── e2e.py          # Coverage-based indexer
```
