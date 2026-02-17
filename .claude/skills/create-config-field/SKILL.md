---
name: create-config-field
description: Add a new configuration field to the Datadog Agent (datadog.yaml)
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[config.key.name]"
---

Add a new configuration field to a Datadog Agent. This involves registering the key with defaults/env bindings in Go, and optionally documenting it in the config template.

There are **two separate config objects**, each with their own Go init function. Within the core agent config, many **subsystems** have dedicated setup functions and template sections.

### Config objects

| Config object | Config file | Go init function |
|---|---|---|
| Core Agent | `datadog.yaml` | `InitConfig()` in `pkg/config/setup/config.go` |
| System Probe | `system-probe.yaml` | `InitSystemProbeConfig()` in `pkg/config/setup/system_probe.go` |

### Core Agent subsystems

The core agent config is organized into subsystem setup functions. Each has its own template conditional for documentation. When adding a field, register it in the right subsystem function and document it under the matching template section.

| Subsystem | Go setup function | Template conditional | Key prefix examples |
|---|---|---|---|
| Common / General | `agent()` or inline in `InitConfig()` | `.Common`, `.Agent` | `api_key`, `hostname`, `site` |
| Logs Agent | `logsagent()` | `.LogsAgent` | `logs_config.*`, `logs_enabled` |
| APM / Trace Agent | `setupAPM()` in `apm.go` | `.TraceAgent` | `apm_config.*` |
| Process Agent | `setupProcesses()` in `process.go` | `.ProcessAgent` | `process_config.*` |
| DogStatsD | `dogstatsd()` | `.Dogstatsd` | `dogstatsd_*`, `statsd_*` |
| Security Agent | via `InitConfig()` | `.SecurityAgent`, `.Compliance` | `security_agent.*`, `compliance_config.*` |
| Cluster Agent | via `InitConfig()` | `.ClusterAgent` | `cluster_agent.*` |
| OTLP | `OTLP()` in `otlp.go` | `.OTLP` | `otlp_config.*` |
| Network Path | via `InitConfig()` | `.NetworkPath` | `network_path.*` |

### System Probe subsystems

| Subsystem | Go setup function | Template conditional | Key prefix examples |
|---|---|---|---|
| General | `InitSystemProbeConfig()` in `system_probe.go` | `.SystemProbe` | `system_probe_config.*` |
| CWS | `initCWSSystemProbeConfig()` in `system_probe_cws.go` | `.SecurityModule` | `runtime_security_config.*` |
| USM | `initUSMSystemProbeConfig()` in `system_probe_usm.go` | `.UniversalServiceMonitoringModule` | `service_monitoring_config.*` |
| Network | via `InitSystemProbeConfig()` | `.NetworkModule` | `network_config.*` |

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the config key name, skip that question.

1. **Target config**: Which config object is this field for?
   - **Core Agent** (`datadog.yaml`) — the main agent config, most common case. Also used by Security Agent, Cluster Agent, and other agents that share the core config.
   - **System Probe** (`system-probe.yaml`) — system-level eBPF monitoring, has a completely separate config object and init function (`InitSystemProbeConfig`).

2. **Subsystem**: Which subsystem does this field belong to? This determines which Go setup function to add it to and which template conditional to use for documentation. Refer to the tables above. If unsure, look at related existing config keys using `Grep` to find which function they're registered in.

2. **Config key** (dot-separated, e.g. `my_feature.enabled`, `logs_config.use_compression`): the YAML path in the config file.

3. **Value type**: What type of value does this field hold?
   - Boolean (`true`/`false`)
   - String
   - Integer
   - Float
   - Duration (e.g. `30 * time.Second`)
   - String slice (list of strings)
   - Map (e.g. `map[string]interface{}`)

4. **Default value**: What should the default be?

5. **Description**: A human-readable description of what this config field controls.

6. **Scope**: Where should this field be registered?
   - **Inline** — add directly in the appropriate init function (small additions)
   - **Dedicated setup file** — for a group of related fields belonging to a feature (`pkg/config/setup/<feature>.go`)
   - **Existing setup file** — to add to an already existing feature's setup file

7. **Serverless-compatible?** (only relevant for Core Agent fields): Is this config reachable by the serverless agent?
   - **Yes** — register via the `serverlessConfigComponents` slice (a function in `pkg/config/setup/config.go`)
   - **No** (default) — register directly in `InitConfig`

8. **User-facing?**: Should this field be documented in the config template (`pkg/config/config_template.yaml`)?
   - **Yes** — add `@param`/`@env` documentation to the template
   - **No** (default for internal/hidden fields) — skip template documentation

9. **Custom env var name?**: By default, `BindEnvAndSetDefault` auto-generates `DD_<KEY_UPPERCASED>` (dots become underscores). Does it need a custom env var name override?

### Step 2: Register the config key

Based on the target config and subsystem, add the field to the appropriate Go function.

#### Option A: Add to an existing subsystem function

This is the most common case. Find the right function from the tables above and add the binding there. For example:
- For a `logs_config.*` field: add to `logsagent()` in `config.go`
- For an `apm_config.*` field: add to `setupAPM()` in `apm.go`
- For a `system_probe_config.*` field: add to `InitSystemProbeConfig()` in `system_probe.go`
- For a `runtime_security_config.*` field: add to `initCWSSystemProbeConfig()` in `system_probe_cws.go`

```go
config.BindEnvAndSetDefault("my_feature.enabled", false)
```

#### Option B: Add inline in the main init function

For standalone fields that don't belong to a specific subsystem, add directly in:
- `InitConfig()` in `config.go` — for core agent fields
- `InitSystemProbeConfig()` in `system_probe.go` — for system-probe fields

Place it near related fields (look for a logical grouping).

#### Option C: Dedicated setup file

For a **group of related fields** belonging to a new feature, create a new file `pkg/config/setup/<feature>.go`:

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	// FeatureEnabled is the config key for enabling the feature
	FeatureEnabled = "my_feature.enabled"
	// FeatureTimeout is the config key for the feature timeout
	FeatureTimeout = "my_feature.timeout"
)

// setupMyFeature registers all configuration keys for my feature
func setupMyFeature(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault(FeatureEnabled, false)
	config.BindEnvAndSetDefault(FeatureTimeout, 30)
}
```

Then call the function from the appropriate place:
- **Core agent, serverless-compatible**: add to the `serverlessConfigComponents` slice in `config.go`
- **Core agent, not serverless-compatible**: call directly inside `InitConfig` in `config.go`
- **System probe**: call from `InitSystemProbeConfig` in `system_probe.go`

### Step 3: (Optional) Document in the config template

If the field is **user-facing**, add documentation to `pkg/config/config_template.yaml`.

The template uses Go template conditionals to control which agent components see each section. Check the `context` struct in `pkg/config/render_config.go` for available conditionals. The most common ones are:

| Conditional | Included in |
|---|---|
| `.Common` | All agents (agent, dogstatsd, cluster-agent, etc.) |
| `.Agent` | Core agent only |
| `.LogsAgent` | Agents with log collection |
| `.TraceAgent` | APM trace agent |
| `.ProcessAgent` | Process agent |
| `.SystemProbe` | System probe |
| `.ClusterAgent` | Cluster agent |
| `.Dogstatsd` | DogStatsD |
| `.SecurityAgent` | Security agent |

**Documentation format:**

For an optional field with default:
```yaml
## @param my_feature.enabled - boolean - optional - default: false
## @env DD_MY_FEATURE_ENABLED - boolean - optional - default: false
## Description of what this field controls.
## Additional details if needed.
#
# my_feature.enabled: false
```

For a required field:
```yaml
## @param my_feature.api_endpoint - string - required
## @env DD_MY_FEATURE_API_ENDPOINT - string - required
## The API endpoint for my feature.
#
# my_feature.api_endpoint:
```

For a field with a nested object:
```yaml
## @param my_feature - custom object - optional
## Configuration for my feature.
#
# my_feature:
#   enabled: false
#   timeout: 30
```

**Type names for `@param`/`@env`:** `boolean`, `string`, `integer`, `number`, `list of strings`, `custom object`

Place the documentation inside the appropriate conditional block. For example, if it applies to all agents, place it inside `{{- if .Common -}}...{{ end -}}`. If it's agent-specific, place it inside `{{ if .Agent }}...{{ end -}}`.

### Step 4: Accessing the config in code

Show the user how to read the new config value. The accessor method depends on the type:

| Type | Accessor |
|---|---|
| Boolean | `config.GetBool("my_feature.enabled")` |
| String | `config.GetString("my_feature.name")` |
| Integer | `config.GetInt("my_feature.timeout")` |
| Float | `config.GetFloat64("my_feature.ratio")` |
| Duration | `config.GetDuration("my_feature.interval")` |
| String slice | `config.GetStringSlice("my_feature.items")` |
| Map | `config.GetStringMap("my_feature.options")` |

The config object is typically accessed via:
- `pkgconfig.Datadog()` from `pkg/config` (global access)
- A `config.Component` dependency injected via Fx

If a constant was defined for the key, remind the user to use it instead of a raw string.

### Step 5: Verify

1. Build to check for compilation errors:
   - Core agent: `dda inv agent.build --build-exclude=systemd`
   - System probe: `dda inv system-probe.build`

2. Run the linter:
   ```bash
   dda inv linter.go
   ```

3. If the config template was modified, verify it renders correctly:
   ```bash
   dda inv generate-config
   ```

4. Report the results to the user. If the build or linting fails, fix the issues.

## Key Methods Reference

The `pkgconfigmodel.Setup` interface provides these methods for registering config keys:

- **`BindEnvAndSetDefault(key, default, envVars...)`** — Preferred. Registers the key as known, sets a default value, and binds the auto-generated `DD_*` env var (plus optional custom env vars).
- **`SetDefault(key, value)`** — Sets a default without env var binding.
- **`BindEnv(key, envVars...)`** — Binds env vars without setting a default.
- **`SetKnown(key)`** — Deprecated. Only marks the key as known. Prefer `BindEnvAndSetDefault`.

**Environment variable naming:** `BindEnvAndSetDefault("my_feature.timeout", 30)` automatically creates `DD_MY_FEATURE_TIMEOUT`. To add a custom alias: `BindEnvAndSetDefault("my_feature.timeout", 30, "DD_MY_TIMEOUT")`.

## Important Notes

- Keys registered with `BindEnvAndSetDefault` are automatically "known" — the agent's validation system won't warn about them at startup.
- The config priority order is: `default < file < environment-variable < fleet-policies < agent-runtime < remote-config < cli`. Higher priority sources override lower ones.
- Always define **exported string constants** for config keys when creating a dedicated setup file. This prevents typos and makes the keys greppable.
- Do NOT use `SetKnown` for new fields — it's deprecated. Use `BindEnvAndSetDefault` instead.

## Usage

- `/create-config-field` — Interactive: prompts for all details
- `/create-config-field my_feature.enabled` — Pre-fills the key name, prompts for the rest
