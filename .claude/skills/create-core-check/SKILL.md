---
name: create-core-check
description: Create a new Go core check that collects metrics and sends them to Datadog
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[check-name]"
---

Create a new Go-based core check for the Datadog Agent. Core checks collect metrics, service checks, or events and send them to Datadog at regular intervals.

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the check name, skip that question.

1. **Check name**: The identifier for the check (e.g. `uptime`, `memory`, `ntp`). Used as the package name, registration key, and config directory name.

2. **Check category**: Where should the check live under `pkg/collector/corechecks/`?
   - `system/` — System-level checks (CPU, memory, uptime, disk)
   - `net/` — Network checks (NTP, DNS)
   - `containers/` — Container-related checks
   - `ebpf/` — eBPF-based checks (these are more complex, see `pkg/collector/corechecks/ebpf/AGENTS.md`)
   - `embed/` — Embedded service checks
   - Top-level under `corechecks/` — For standalone checks

3. **What does it collect?**: Describe the metrics, service checks, or events it produces.

4. **Configuration**: Does it need instance-level configuration?
   - **No config** — Single instance, no user parameters (like `uptime`)
   - **Simple config** — A few YAML parameters (like `memory` with `collect_memory_pressure`)
   - **Multi-instance** — Supports multiple configured instances (like `ntp` with different servers)

5. **Component dependencies**: Does the check need injected components?
   - **None** — Simple check, no external dependencies
   - **Tagger** — Needs to tag metrics with container/host tags
   - **WorkloadMeta** — Needs access to workload metadata store
   - **Other** — Specify which components

6. **Long-running?**: Does the check run continuously in the background?
   - **No** (default) — `Run()` is called at regular intervals (default 15s)
   - **Yes** — `Run()` never returns, processes events in a loop

7. **Platform restrictions**: Does the check only work on certain platforms?
   - All platforms (default)
   - Linux only
   - Windows only
   - Linux + macOS (not Windows)

### Step 2: Read reference examples from the codebase

Before writing any code, read the appropriate reference files based on the check type determined in Step 1. Follow the patterns found in these files exactly.

| Check type | Reference file to read |
|---|---|
| Simple, no config | `pkg/collector/corechecks/system/uptime/uptime.go` |
| Simple with config | `pkg/collector/corechecks/system/memory/memory.go` |
| Multi-instance with config | `pkg/collector/corechecks/net/ntp/ntp.go` |
| With component dependencies | `pkg/collector/corechecks/containerimage/check.go` |
| Long-running | Read the `NewLongRunningCheckWrapper` usage in `pkg/collector/corechecks/containerimage/check.go` |
| Platform-specific stubs | Find a `_no*.go` or `_stub.go` file alongside a platform-specific check in `pkg/collector/corechecks/system/` |

Also read these files for registration and test patterns:
- `pkg/commonchecks/corechecks.go` — to see how checks are registered (import alias convention, `RegisterCheck` calls)
- The `_test.go` file alongside whichever reference check you read — to see mock sender patterns

### Step 3: Create the check package

**Directory:** `pkg/collector/corechecks/<category>/<checkname>/`

Create the check implementation file following the patterns from the reference files read in Step 2. Key structural elements that every check needs:

1. **`CheckName` constant** — string identifier for the check
2. **`Check` struct** — embeds `core.CheckBase`, plus any config or component fields
3. **`Factory()` function** — returns `option.Option[func() check.Check]`. Components are injected as Factory parameters.
4. **`Configure()` method** — calls `CommonConfigure`, then `FinalizeCheckServiceTag`, then parses instance config if needed
5. **`Run()` method** — collects data, calls sender methods, ends with `sender.Commit()`

Key rules to follow:
- For **multi-instance** checks: call `c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)` **before** `CommonConfigure()`
- For **long-running** checks: wrap with `core.NewLongRunningCheckWrapper()` in Factory, return `0` from `Interval()`, implement `Stop()`
- For **platform-specific** checks: add `//go:build <platform>` tag and create a stub file for other platforms that returns `option.None[func() check.Check]()`

### Step 4: Register the check

Edit `pkg/commonchecks/corechecks.go`:

1. Add an import for the check package using the standard alias convention visible in the file (typically the check name)
2. Add a `corecheckLoader.RegisterCheck()` call in `RegisterChecks()`, matching the Factory signature to available component parameters

### Step 5: Create the default configuration

**File:** `cmd/agent/dist/conf.d/<checkname>.d/conf.yaml.default`

Look at an existing example in `cmd/agent/dist/conf.d/` for the format. At minimum:

```yaml
init_config:

instances:
  - {}
```

For checks with configuration, use `@param` annotations following the same format as other `conf.yaml.default` files in the tree.

### Step 6: Write tests

**File:** `pkg/collector/corechecks/<category>/<checkname>/<checkname>_test.go`

Follow the test patterns from the reference file read in Step 2. The standard test flow is:

1. Create a `mocksender.NewMockSender("")`
2. Set up `mockSender.On("FinalizeCheckServiceTag").Return()`
3. Create and `Configure` the check with `mockSender.GetSenderManager()`
4. Call `mocksender.SetSender(mockSender, check.ID())`
5. Set expectations on the mock sender for expected metrics
6. Call `Run()` and assert expectations

### Step 7: Verify

1. Run the check tests:
   ```bash
   dda inv test --targets=./pkg/collector/corechecks/<category>/<checkname>
   ```

2. Build the agent:
   ```bash
   dda inv agent.build --build-exclude=systemd
   ```

3. Run the linter:
   ```bash
   dda inv linter.go
   ```

4. Report the results to the user.

## Sender Methods Reference

The sender (`c.GetSender()`) provides these methods for submitting data:

| Method | Description |
|---|---|
| `Gauge(metric, value, hostname, tags)` | Submit a gauge metric |
| `Rate(metric, value, hostname, tags)` | Submit a rate metric |
| `Count(metric, value, hostname, tags)` | Submit a count metric |
| `MonotonicCount(metric, value, hostname, tags)` | Submit a monotonic count |
| `Histogram(metric, value, hostname, tags)` | Submit a histogram metric |
| `Distribution(metric, value, hostname, tags)` | Submit a distribution metric |
| `ServiceCheck(name, status, hostname, tags, message)` | Submit a service check |
| `Event(event)` | Submit an event |
| `Commit()` | Flush all submitted data — **must be called at end of Run()** |

- Pass `""` for hostname to use the agent's default hostname.
- Pass `nil` for tags if no tags are needed.
- Service check statuses: `servicecheck.ServiceCheckOK`, `ServiceCheckWarning`, `ServiceCheckCritical`, `ServiceCheckUnknown` (from `pkg/metrics/servicecheck`).

## Important Notes

- `CheckBase` provides default implementations for most `Check` interface methods. You only need to override `Run()` and optionally `Configure()`, `Stop()`, and `Interval()`.
- `CommonConfigure` handles standard configuration: collection interval (`min_collection_interval`), custom tags, service tag, etc.
- `FinalizeCheckServiceTag()` must be called after `CommonConfigure` to apply the service tag to the sender.
- Always call `sender.Commit()` at the end of `Run()` to flush data.
- For multi-instance checks, `BuildID()` must be called **before** `CommonConfigure()`.
- The `option.None[func() check.Check]()` pattern is used for platform stubs — the loader skips checks with no factory.
- `integration.FakeConfigHash` is the constant to use in tests for the config digest parameter.

## Usage

- `/create-core-check` — Interactive: prompts for all details
- `/create-core-check my_check` — Pre-fills the check name
