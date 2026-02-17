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

### Step 2: Create the check package

**Directory:** `pkg/collector/corechecks/<category>/<checkname>/`

#### Basic check (no config, no dependencies)

**File:** `pkg/collector/corechecks/<category>/<checkname>/<checkname>.go`

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package <checkname>

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const CheckName = "<checkname>"

// Check implements the <checkname> check.
type Check struct {
	core.CheckBase
}

// Factory returns the check factory.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
	}
}

// Configure configures the check.
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()

	return nil
}

// Run executes the check.
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// Collect and send metrics
	// sender.Gauge("my_check.metric_name", value, "", nil)

	sender.Commit()
	return nil
}
```

#### Check with instance configuration

Add a config struct and parse it in `Configure`:

```go
import "gopkg.in/yaml.v2"

type instanceConfig struct {
	MyParam    string   `yaml:"my_param"`
	SomeFlag   bool     `yaml:"some_flag"`
	ItemList   []string `yaml:"item_list"`
}

type Check struct {
	core.CheckBase
	config instanceConfig
}

func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()

	return yaml.Unmarshal(rawInstance, &c.config)
}
```

#### Multi-instance check

Call `BuildID` **before** `CommonConfigure` to create a unique ID per instance:

```go
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()

	return yaml.Unmarshal(rawInstance, &c.config)
}
```

#### Check with component dependencies

Components are injected via the Factory function:

```go
import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type Check struct {
	core.CheckBase
	tagger tagger.Component
	store  workloadmeta.Component
}

func Factory(tagger tagger.Component, store workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase: core.NewCheckBase(CheckName),
			tagger:    tagger,
			store:     store,
		}
	})
}
```

#### Long-running check

For checks that process events continuously:

```go
import core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"

type Check struct {
	core.CheckBase
	stopCh chan struct{}
}

func Factory() option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return core.NewLongRunningCheckWrapper(&Check{
			CheckBase: core.NewCheckBase(CheckName),
			stopCh:    make(chan struct{}),
		})
	})
}

func (c *Check) Run() error {
	// This never returns — runs until Stop() is called
	for {
		select {
		case <-c.stopCh:
			return nil
		case event := <-someEventChannel:
			sender, _ := c.GetSender()
			// Process event, send metrics
			sender.Commit()
		}
	}
}

func (c *Check) Stop() {
	close(c.stopCh)
}

func (c *Check) Interval() time.Duration {
	return 0 // 0 signals long-running check
}
```

#### Platform-specific check

Add build tags and create stub files for unsupported platforms:

**Main file** (`<checkname>.go`):
```go
//go:build linux

package <checkname>
// ... full implementation
```

**Stub file** (`<checkname>_nolinux.go`):
```go
//go:build !linux

package <checkname>

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const CheckName = "<checkname>"

// Factory returns an empty Option on unsupported platforms.
func Factory() option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
```

### Step 3: Register the check

Edit `pkg/commonchecks/corechecks.go`:

1. Add an import for the check package:
   ```go
   <checkname> "github.com/DataDog/datadog-agent/pkg/collector/corechecks/<category>/<checkname>"
   ```

2. Add a `RegisterCheck` call in the `RegisterChecks` function:
   ```go
   // Simple check (no dependencies)
   corecheckLoader.RegisterCheck(<checkname>.CheckName, <checkname>.Factory())

   // Check with dependencies
   corecheckLoader.RegisterCheck(<checkname>.CheckName, <checkname>.Factory(tagger, store))
   ```

   Match the Factory arguments to the component parameters available in `RegisterChecks`.

### Step 4: Create the default configuration

**File:** `cmd/agent/dist/conf.d/<checkname>.d/conf.yaml.default`

For a check with no user configuration:
```yaml
init_config:

instances:
  - {}
```

For a check with configuration:
```yaml
init_config:

instances:
    ## @param my_param - string - optional - default: ""
    ## Description of what my_param does.
    #
    # - my_param: ""

    ## @param some_flag - boolean - optional - default: false
    ## Description of some_flag.
    #
    # - some_flag: false
```

### Step 5: Write tests

**File:** `pkg/collector/corechecks/<category>/<checkname>/<checkname>_test.go`

```go
package <checkname>

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestRun(t *testing.T) {
	// Create mock sender before Configure
	mockSender := mocksender.NewMockSender("")
	mockSender.On("FinalizeCheckServiceTag").Return()

	// Create and configure check
	c := new(Check)
	err := c.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	require.NoError(t, err)

	// Register mock sender with the check's ID
	mocksender.SetSender(mockSender, c.ID())

	// Set expectations for metrics
	mockSender.On("Gauge", "my_check.metric", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	mockSender.On("Commit").Return().Times(1)

	// Run
	err = c.Run()
	require.NoError(t, err)

	// Verify
	mockSender.AssertExpectations(t)
}

func TestConfigure(t *testing.T) {
	mockSender := mocksender.NewMockSender("")
	mockSender.On("FinalizeCheckServiceTag").Return()

	c := new(Check)
	rawInstance := []byte(`
my_param: "test_value"
some_flag: true
`)
	err := c.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstance, nil, "test")
	require.NoError(t, err)
	assert.Equal(t, "test_value", c.config.MyParam)
	assert.True(t, c.config.SomeFlag)
}
```

### Step 6: Verify

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
- `CommonConfigure` (called via `c.CommonConfigure(...)`) handles standard configuration: collection interval (`min_collection_interval`), custom tags, service tag, etc.
- `FinalizeCheckServiceTag()` must be called after `CommonConfigure` to apply the service tag to the sender.
- Always call `sender.Commit()` at the end of `Run()` to flush data.
- For multi-instance checks, `BuildID()` must be called **before** `CommonConfigure()`.
- The `option.None[func() check.Check]()` pattern is used for platform stubs — the loader skips checks with no factory.
- `integration.FakeConfigHash` is the constant to use in tests for the config digest parameter.

## Usage

- `/create-core-check` — Interactive: prompts for all details
- `/create-core-check my_check` — Pre-fills the check name
