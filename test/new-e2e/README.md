# New E2E tests

The new E2E tests aim to have a complete test infrastructure deployed on every E2E pipeline.

## Setup

The E2E tests rely on `pulumi` (installation instructions [here](https://www.pulumi.com/docs/get-started/install/)), it is used to deploy and destroy cloud infrastructure.

## Running E2E tests locally

`dda inv new-e2e-tests.run --targets <test folder path> --osversion '<comma separated os version list>' --platform '<debian/centos/suse/ubuntu>' --arch <x86_64/arm64>`

For the full list of parameters and information on how to run E2E tests locally you can check the task definition [`tasks/new_e2e_tests.py`](../../tasks/new_e2e_tests.py).

## Pre-built Test Binaries (Optimization)

To improve CI pipeline performance, the E2E testing system supports pre-building test binaries that can be reused across multiple test jobs. This eliminates the need to compile tests for each job, significantly reducing pipeline execution time.

### How it works

1. **Build Phase**: A dedicated job `go_e2e_test_binaries` runs early in the pipeline and compiles all E2E test packages into binary executables.

2. **Artifact Storage**: The compiled binaries are stored as GitLab CI artifacts along with a manifest file containing metadata.

3. **Test Execution**: Test jobs can use the pre-built binaries instead of compiling on-the-fly by using the `--use-prebuilt-binaries` flag.

### Building Test Binaries

To build test binaries locally:

```bash
# Build all E2E test binaries
dda inv new-e2e-tests.build-binaries

# Build with custom output directory and tags
dda inv new-e2e-tests.build-binaries --output-dir my-test-binaries --tags "test,integration"
```

This creates:
- `test-binaries/` directory containing compiled `.test` files
- `test-binaries/manifest.json` with build metadata and binary information

### Using Pre-built Binaries

To run tests with pre-built binaries:

```bash
# Use pre-built binaries from default directory
dda inv new-e2e-tests.run --use-prebuilt-binaries --targets ./tests/containers

# Use pre-built binaries from custom directory
dda inv new-e2e-tests.run --use-prebuilt-binaries --binaries-dir my-test-binaries --targets ./tests/agent-subcommands
```

### GitLab CI Integration

In GitLab CI, use the `.new_e2e_template_with_prebuilt_binaries` template for optimized test execution:

```yaml
my-optimized-e2e-test:
  extends: .new_e2e_template_with_prebuilt_binaries
  needs:
    - !reference [.needs_new_e2e_template_with_binaries]
    - agent_deb-x64-a7  # Add your package dependencies
  variables:
    TARGETS: ./tests/my-test-package
    TEAM: my-team
```

### Benefits

- **Faster CI**: Eliminates test compilation time in each job
- **Consistent Builds**: All tests use identical compiled binaries
- **Resource Efficiency**: Reduces CPU and memory usage in test jobs
- **Parallel Execution**: Multiple test jobs can run simultaneously without rebuild overhead

### Manifest File Structure

The manifest file contains:

```json
{
  "build_info": {
    "timestamp": "2024-01-01T12:00:00Z",
    "commit": "abc123",
    "build_tags": "test"
  },
  "binaries": [
    {
      "package": "test/new-e2e/tests/containers",
      "binary": "test-new-e2e-tests-containers.test",
      "size": 52428800
    }
  ]
}
```

This optimization is particularly useful for large test suites and when running multiple test jobs in parallel.
