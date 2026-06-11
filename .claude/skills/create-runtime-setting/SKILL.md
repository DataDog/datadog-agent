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

### Step 2: Read reference examples from the codebase

Before writing any code, read the appropriate reference files to follow existing patterns exactly.

1. **Read the interface** defined in `comp/core/settings/component.go` to understand the `RuntimeSetting` methods.

2. **Read an existing implementation** matching the chosen value type. Use `Glob` with pattern `pkg/config/settings/runtime_setting_*.go` to list available examples, then read one that matches the desired type (boolean, integer, string, etc.).

3. **Read the test file** alongside the chosen reference to see the test pattern.

4. **Read a registration site**: Look at one of the `command.go` files listed in Step 1.7 to see how settings are added to the `Settings` map.

### Step 3: Create the RuntimeSetting implementation file

**File naming convention**: `runtime_setting_<feature_name>.go`

**File location**:
- Shared: `pkg/config/settings/runtime_setting_<feature>.go`
- Agent-specific: `cmd/agent/subcommands/run/internal/settings/runtime_setting_<feature>.go`

Create the implementation following the patterns from the reference file read in Step 2. Every RuntimeSetting needs:

1. **Struct** with a `ConfigKey string` field
2. **Constructor** `New<Name>RuntimeSetting()` that sets the config key
3. **`Description()`** — returns the human-readable description
4. **`Hidden()`** — returns whether hidden from list-runtime
5. **`Name()`** — returns the config key
6. **`Get(config)`** — reads the current value using the appropriate typed getter
7. **`Set(config, v, source)`** — validates/converts the input value, then calls `config.Set()`

**Type conversion in Set()**: for Boolean and Integer types, use the `GetBool(v)` / `GetInt(v)` helper functions from `pkg/config/settings` — these handle string-to-type conversion. For agent-specific settings, import the helpers via `settings "github.com/DataDog/datadog-agent/pkg/config/settings"`.

### Step 4: Create a unit test file

Create a test file alongside the implementation: `runtime_setting_<feature>_test.go`

Follow the test patterns from the reference test file read in Step 2. The test should verify:
- `Name()`, `Description()`, `Hidden()` return expected values
- `Get` returns the correct value from config
- `Set` with a valid value updates the config
- `Set` with a string representation works (e.g. `"true"`/`"false"` for bools)
- `Set` with an invalid value returns an error

### Step 5: Register the setting

Find the `settings.Params` provider in the appropriate `command.go` file(s) for each selected service (from Step 1.7). Add the new setting to the `Settings` map following the existing pattern in that file. The import alias convention and registration format are visible in the existing entries.

### Step 6: Verify

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
