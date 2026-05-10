---
name: create-subcommand
description: Add a new CLI subcommand to an agent binary (agent, cluster-agent, etc.)
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[agent] [subcommand-name]"
---

Add a new CLI subcommand to a Datadog Agent binary. Each agent binary uses Cobra commands registered via a central `subcommands.go` file.

## Agent Binaries

| Agent | Subcommands dir | Registration file |
|---|---|---|
| Core Agent | `cmd/agent/subcommands/` | `cmd/agent/subcommands/subcommands.go` |
| Cluster Agent | `cmd/cluster-agent/subcommands/` | `cmd/cluster-agent/subcommands/subcommands.go` |
| Process Agent | `cmd/process-agent/subcommands/` | `cmd/process-agent/subcommands/subcommands.go` |
| Security Agent | `cmd/security-agent/subcommands/` | `cmd/security-agent/subcommands/subcommands.go` |
| System Probe | `cmd/system-probe/subcommands/` | `cmd/system-probe/subcommands/subcommands.go` |
| DogStatsD | `cmd/dogstatsd/subcommands/` | `cmd/dogstatsd/subcommands/subcommands.go` |

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides values, skip those questions.

1. **Target agent**: Which agent binary? (Core Agent is most common)
2. **Subcommand name**: e.g. `health`, `hostname`, `flare`
3. **Description**: What does the subcommand do?
4. **Complexity**: Simple (no Fx, e.g. `version`) or with config/IPC (`fxutil.OneShot`, e.g. `hostname`)?
5. **Shared across agents?**: Yes → `pkg/cli/subcommands/<name>/`, No → `cmd/<agent>/subcommands/<name>/`

### Step 2: Read reference examples

Read the reference matching the chosen pattern, plus the target agent's registration file:

| Pattern | Reference file |
|---|---|
| Simple (no Fx) | `cmd/agent/subcommands/version/command.go` |
| With config/IPC | `cmd/agent/subcommands/hostname/command.go` |
| Shared across agents | `pkg/cli/subcommands/health/command.go` + `cmd/agent/subcommands/health/command.go` |

Also read the target agent's `command/command.go` for available `GlobalParams` fields.

### Step 3: Create the subcommand

Create `cmd/<agent>/subcommands/<name>/command.go` following the reference patterns exactly.

### Step 4: Register

Edit the agent's `subcommands.go`: add an import with the `cmd<Name>` alias convention and add `cmd<Name>.Commands` to the factory slice. Follow the existing entries.

### Step 5: Verify

1. Build: `dda inv agent.build --build-exclude=systemd`
2. Test: `./bin/agent/agent <name> --help`
3. Lint: `dda inv linter.go`

## Usage

- `/create-subcommand` — Interactive: prompts for all details
- `/create-subcommand agent my-command` — Pre-fills agent and name
