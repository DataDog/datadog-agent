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

| Subsystem | Go setup function | Template conditional | Key prefix examples |
|---|---|---|---|
| Common / General | `agent()` or inline in `InitConfig()` | `.Common`, `.Agent` | `api_key`, `hostname`, `site` |
| Logs Agent | `logsagent()` | `.LogsAgent` | `logs_config.*` |
| APM / Trace Agent | `setupAPM()` in `apm.go` | `.TraceAgent` | `apm_config.*` |
| Process Agent | `setupProcesses()` in `process.go` | `.ProcessAgent` | `process_config.*` |
| DogStatsD | `dogstatsd()` | `.Dogstatsd` | `dogstatsd_*` |
| Security Agent | via `InitConfig()` | `.SecurityAgent` | `security_agent.*` |
| Cluster Agent | via `InitConfig()` | `.ClusterAgent` | `cluster_agent.*` |
| OTLP | `OTLP()` in `otlp.go` | `.OTLP` | `otlp_config.*` |

### System Probe subsystems

| Subsystem | Go setup function | Template conditional | Key prefix examples |
|---|---|---|---|
| General | `InitSystemProbeConfig()` | `.SystemProbe` | `system_probe_config.*` |
| CWS | `initCWSSystemProbeConfig()` in `system_probe_cws.go` | `.SecurityModule` | `runtime_security_config.*` |
| USM | `initUSMSystemProbeConfig()` in `system_probe_usm.go` | `.UniversalServiceMonitoringModule` | `service_monitoring_config.*` |
| Network | via `InitSystemProbeConfig()` | `.NetworkModule` | `network_config.*` |

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the config key name, skip that question.

1. **Target config**: Core Agent (`datadog.yaml`) or System Probe (`system-probe.yaml`)?
2. **Subsystem**: Which subsystem does this field belong to? (see tables above)
3. **Config key** (dot-separated, e.g. `my_feature.enabled`): the YAML path.
4. **Value type**: Boolean, String, Integer, Float, Duration, String slice, or Map.
5. **Default value**: What should the default be?
6. **Description**: Human-readable description of what this field controls.
7. **Scope**: Add inline in the subsystem function, or create a dedicated setup file (`pkg/config/setup/<feature>.go`) for a group of related fields?
8. **User-facing?**: Should it be documented in `pkg/config/config_template.yaml`?

### Step 2: Register the config key

Find the right subsystem function from the tables above and add the binding. Use `Grep` if unsure where related keys live.

```go
config.BindEnvAndSetDefault("my_feature.enabled", false)
```

For a **group of related fields**, create `pkg/config/setup/<feature>.go` with a dedicated setup function, then call it from the appropriate init function. Read an existing setup file (e.g. `pkg/config/setup/apm.go`) for the pattern.

For **serverless-compatible** core agent fields, register via the `serverlessConfigComponents` slice in `config.go` instead of directly in `InitConfig`.

### Step 3: (Optional) Document in the config template

If user-facing, add documentation to `pkg/config/config_template.yaml` inside the appropriate conditional block (see Template conditional column in tables above). Read existing entries nearby for the exact format (`@param`, `@env` annotations).

### Step 4: Verify

1. Build: `dda inv agent.build --build-exclude=systemd` (or `dda inv system-probe.build`)
2. Lint: `dda inv linter.go`
3. If the template was modified: `dda inv generate-config`
4. Report the results to the user.

## Key Methods Reference

- **`BindEnvAndSetDefault(key, default, envVars...)`** — Preferred. Registers key, sets default, binds `DD_*` env var.
- **`SetDefault(key, value)`** — Default without env binding.
- **`BindEnv(key, envVars...)`** — Env binding without default.

`BindEnvAndSetDefault("my_feature.timeout", 30)` auto-creates `DD_MY_FEATURE_TIMEOUT`. Custom alias: `BindEnvAndSetDefault("my_feature.timeout", 30, "DD_MY_TIMEOUT")`.

## Important Notes

- Do NOT use `SetKnown` for new fields — it's deprecated. Use `BindEnvAndSetDefault`.
- Config priority: `default < file < env-var < fleet-policies < agent-runtime < remote-config < cli`.
- Define exported string constants for keys when creating a dedicated setup file.

## Usage

- `/create-config-field` — Interactive: prompts for all details
- `/create-config-field my_feature.enabled` — Pre-fills the key name
