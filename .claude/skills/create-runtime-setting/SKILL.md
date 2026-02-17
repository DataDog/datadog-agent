---
name: create-runtime-setting
description: Create a new RuntimeSetting that can be changed at runtime via `agent config set/get` and the config API
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[setting-name]"
---

Create a new RuntimeSetting implementation for the Datadog Agent. RuntimeSettings are settings that can be read and changed at runtime via:
- The CLI: `agent config get <setting>`, `agent config set <setting> <value>`, `agent config list-runtime`
- The HTTP API: `GET /config/{setting}`, `POST /config/{setting}`

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the setting name, skip that question.

1. **Setting name** (the config key, e.g. `log_payloads`, `dogstatsd_stats`): the name used to register and access the setting via the API.

2. **Value type**: What type of value does this setting hold?
   - Boolean (true/false)
   - Integer
   - String
   - String slice (list of strings)

3. **Description**: A human-readable description of what this setting controls (shown in `/config/list-runtime`).

4. **Hidden**: Should this setting be hidden from the public runtime settings list? (default: false)

5. **Scope**: Where should this setting live?
   - **Shared** (`pkg/config/settings/`) — Used by multiple agent services (agent, trace-agent, process-agent, etc.)
   - **Agent-specific** (`cmd/agent/subcommands/run/internal/settings/`) — Only used by the core agent

6. **Config key**: The `datadog.yaml` config key this setting maps to (e.g. `log_payloads`, `internal_profiling.enabled`). Often the same as the setting name, but can differ.

7. **Which services should register it**: Ask which services should have this setting registered:
   - Core Agent (`cmd/agent/subcommands/run/command.go`)
   - Cluster Agent (`cmd/cluster-agent/subcommands/start/command.go`)
   - Trace Agent (`cmd/trace-agent/subcommands/run/command.go`)
   - Process Agent (`cmd/process-agent/subcommands/run/command.go`)
   - Security Agent (`cmd/security-agent/subcommands/runtime/command.go`)
   - System Probe (`cmd/system-probe/subcommands/run/command.go`)
   - DogStatsD (`cmd/dogstatsd/subcommands/start/command.go`)

### Step 2: Create the RuntimeSetting implementation file

Based on the collected information, create the implementation file.

**File naming convention**: `runtime_setting_<feature_name>.go`

**File location**:
- Shared: `pkg/config/settings/runtime_setting_<feature>.go`
- Agent-specific: `cmd/agent/subcommands/run/internal/settings/runtime_setting_<feature>.go`

Use this template, adapting it based on the value type:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// <StructName>RuntimeSetting wraps operations to change <description> at runtime.
type <StructName>RuntimeSetting struct {
	ConfigKey string
}

// New<StructName>RuntimeSetting returns a new <StructName>RuntimeSetting
func New<StructName>RuntimeSetting() *<StructName>RuntimeSetting {
	return &<StructName>RuntimeSetting{ConfigKey: "<config_key>"}
}

// Description returns the runtime setting's description
func (s *<StructName>RuntimeSetting) Description() string {
	return "<description>"
}

// Hidden returns whether this setting is hidden from the list of runtime settings
func (s *<StructName>RuntimeSetting) Hidden() bool {
	return <true|false>
}

// Name returns the name of the runtime setting
func (s *<StructName>RuntimeSetting) Name() string {
	return s.ConfigKey
}

// Get returns the current value of the runtime setting
func (s *<StructName>RuntimeSetting) Get(config config.Component) (interface{}, error) {
	return config.Get<Type>(s.ConfigKey), nil
}

// Set changes the value of the runtime setting
func (s *<StructName>RuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	var newValue <type>
	var err error

	if newValue, err = Get<Type>(v); err != nil {
		return fmt.Errorf("<StructName>RuntimeSetting: %v", err)
	}

	config.Set(s.ConfigKey, newValue, source)
	return nil
}
```

**Type-specific patterns for Set/Get**:

- **Boolean**: Use `GetBool(v)` helper from `pkg/config/settings`, `config.GetBool(key)` for Get
- **Integer**: Use `GetInt(v)` helper from `pkg/config/settings`, `config.GetInt(key)` for Get
- **String**: Use type assertion `v.(string)`, `config.GetString(key)` for Get
- **String slice**: Use `config.GetStringSlice(key)` for Get

For **agent-specific** settings, the import path for the helper functions changes:
```go
import (
    settings "github.com/DataDog/datadog-agent/pkg/config/settings"
)
// Then use: settings.GetBool(v), settings.GetInt(v)
```

### Step 3: Create a unit test file

Create a test file alongside the implementation: `runtime_setting_<feature>_test.go`

The test should verify:
- `Get` returns the correct value from config
- `Set` with a valid value updates the config
- `Set` with a string representation works (e.g. "true"/"false" for bools, string numbers for ints)
- `Set` with an invalid value returns an error
- `Description()` returns the expected string
- `Hidden()` returns the expected value
- `Name()` returns the expected name

For shared settings (in `pkg/config/settings/`), use this simpler test pattern with `configmock`:

```go
package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/comp/core"
)

func Test<StructName>RuntimeSetting(t *testing.T) {
	config := fxutil.Test[config.Component](t, core.MockBundle())
	s := New<StructName>RuntimeSetting()

	// Test metadata
	assert.Equal(t, "<config_key>", s.Name())
	assert.Equal(t, "<description>", s.Description())
	assert.Equal(t, <true|false>, s.Hidden())

	// Test Set and Get
	err := s.Set(config, <valid_value>, model.SourceCLI)
	require.NoError(t, err)
	v, err := s.Get(config)
	require.NoError(t, err)
	assert.Equal(t, <expected_value>, v)

	// Test Set with string representation
	err = s.Set(config, "<string_value>", model.SourceCLI)
	require.NoError(t, err)
	v, err = s.Get(config)
	require.NoError(t, err)
	assert.Equal(t, <expected_value>, v)
}
```

### Step 4: Register the setting

Find the `settings.Params` provider in the appropriate `command.go` file(s) for each selected service. Add the new setting to the `Settings` map:

```go
"<setting_name>": commonsettings.New<StructName>RuntimeSetting(),
```

For shared settings, the import alias is typically `commonsettings`:
```go
commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
```

For agent-specific settings, the import alias is typically `internalsettings`:
```go
internalsettings "github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/settings"
```

### Step 5: Verify

1. Run the new test:
   ```bash
   dda inv test --targets=<package_path>
   ```

2. Run the linter on changed files:
   ```bash
   dda inv linter.go
   ```

3. Report the results to the user. If tests or linting fail, fix the issues.

## Important Notes

- The `RuntimeSetting` interface is defined in `comp/core/settings/component.go`
- Helper functions `GetBool` and `GetInt` are in `pkg/config/settings/runtime_setting.go`
- All `Set` methods receive a `model.Source` parameter for config source tracking — always pass it through to `config.Set()`
- Settings are exposed via HTTP at `/config/{setting_name}` (GET to read, POST to write) and via the CLI: `agent config get <setting>`, `agent config set <setting> <value>`, `agent config list-runtime`
- Follow existing code style: use the same comment patterns, error formatting, and naming conventions as existing RuntimeSettings

## Usage

- `/create-runtime-setting` — Interactive: prompts for all details
- `/create-runtime-setting my_new_setting` — Pre-fills the setting name, prompts for the rest
