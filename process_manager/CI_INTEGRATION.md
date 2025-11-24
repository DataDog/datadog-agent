# GitLab CI Integration for Process Manager

## Overview

The process manager tests are integrated into the Datadog Agent's GitLab CI pipeline under the `source_test` stage.

## Configuration Files

- **`.gitlab/source_test/process_manager.yml`** - Main CI configuration for process manager tests
- **`.gitlab/source_test/include.yml`** - Updated to include process_manager.yml

## CI Jobs

### 1. `process_manager_unit_tests`
**Purpose:** Run unit tests for the process manager engine library

**When it runs:**
- On changes to `process_manager/**/*`
- On fast dev branches
- When unit tests are not disabled

**What it does:**
```bash
cd process_manager
cargo test --lib --workspace --verbose
cargo test -p pm_engine --verbose
```

**Duration:** ~2-5 minutes

---

### 2. `process_manager_e2e_tests`
**Purpose:** Run cross-platform e2e tests (non-Linux-specific features)

**When it runs:**
- On changes to `process_manager/**/*`
- On fast dev branches
- When unit tests are not disabled

**What it tests:**
- Basic process management (create, start, stop, delete)
- Auto-start functionality
- Configuration loading
- Dependencies (requires, wants, after, binds_to, conflicts)
- Health checks (HTTP, TCP, Exec)
- Restart policies
- Process types (simple, forking, oneshot)
- Update operations

**Duration:** ~10-20 minutes

---

### 3. `process_manager_e2e_tests_linux`
**Purpose:** Run Linux-specific e2e tests (capabilities, cgroups, etc.)

**When it runs:**
- On main branch or release branches
- On changes to `process_manager/**/*`
- When unit tests are not disabled

**What it tests:**
- Ambient capabilities (Linux-only)
- Resource limits (cgroups v2)
- Runtime directories
- Orphan process handling
- Socket activation
- Conditional starting (ConditionPathExists)

**Duration:** ~15-25 minutes

---

### 4. `process_manager_smoke_test`
**Purpose:** Quick feedback for fast iteration

**When it runs:**
- When `FAST_TESTS=true`
- On changes to `process_manager/**/*`

**What it tests:**
- Basic startup test
- Basic process operations

**Duration:** ~1-3 minutes

---

## Resource Requirements

All jobs use:
- **CPU:** 4 cores
- **Memory:** 8GB (request and limit)
- **Timeout:** 30 minutes (smoke test: 5 minutes)
- **Platform:** Linux x64

## Artifacts

Test results are saved as JUnit XML for GitLab test reporting:
- **Path:** `process_manager/target/debug/junit-*.xml`
- **Retention:** 2 weeks

## Running Tests Locally

### All tests
```bash
cd process_manager
cargo test --workspace
```

### Unit tests only
```bash
cd process_manager
cargo test --lib --workspace
```

### Specific e2e test
```bash
cd process_manager
cargo test --test e2e_basic --verbose
```

### All e2e tests
```bash
cd process_manager
cargo test -p pm-e2e-tests --verbose
```

## Troubleshooting

### Tests fail with "daemon not found"
**Cause:** Debug binaries not built

**Solution:**
```bash
cd process_manager
cargo build  # Build debug binaries
cargo test
```

### Tests timeout
**Cause:** Tests running serially, or daemon not starting

**Solution:**
- Check daemon logs at `/tmp/daemon-*.log`
- Run with `--test-threads=1` to avoid port conflicts
- Increase timeout in CI configuration

### Rust toolchain not available
**Cause:** CI image doesn't have Rust installed

**Solution:** The `before_script` in `.gitlab/source_test/process_manager.yml` automatically installs Rust using rustup

## CI Pipeline Integration

The process manager tests are part of the `source_test` stage, which runs in parallel with other source tests like:
- Go unit tests
- Python tests
- eBPF tests
- Linting

### Pipeline Stages
```
.pre → source_test → binary_build → package_build → deploy
         ↓
   process_manager_tests
```

## Skipping Tests

To skip process manager tests in a commit:
```bash
git commit -m "Your message [skip process_manager_tests]"
```

## Test Coverage

Current test coverage:
- **23 e2e test files** covering all major features
- **Unit tests** for engine library
- **Integration tests** for component interactions

## Adding New Tests

1. Add test file to `process_manager/tests/e2e_*.rs`
2. Update `.gitlab/source_test/process_manager.yml` to include it
3. Ensure it passes locally before pushing

## Performance Considerations

- Tests use unique ports/sockets to allow parallel execution
- `--test-threads=1` prevents port conflicts in CI
- Debug builds are faster to compile but slower to run
- Release builds used for e2e tests for better performance

## Future Improvements

- [ ] Add test coverage reporting
- [ ] Cache Rust dependencies between runs
- [ ] Run macOS-specific tests on macOS runners
- [ ] Add performance benchmarks to CI
- [ ] Generate test reports with detailed metrics

