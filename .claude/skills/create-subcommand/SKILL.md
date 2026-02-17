---
name: create-subcommand
description: Add a new CLI subcommand to an agent binary (agent, cluster-agent, etc.)
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[agent] [subcommand-name]"
---

Add a new CLI subcommand to a Datadog Agent binary. Each agent binary uses Cobra commands registered via a central `subcommands.go` file.

## Agent Binaries & Their Command Packages

| Agent | Command package | Subcommands dir | Registration file |
|---|---|---|---|
| Core Agent | `cmd/agent/command` | `cmd/agent/subcommands/` | `cmd/agent/subcommands/subcommands.go` |
| Cluster Agent | `cmd/cluster-agent/command` | `cmd/cluster-agent/subcommands/` | `cmd/cluster-agent/subcommands/subcommands.go` |
| Process Agent | `cmd/process-agent/command` | `cmd/process-agent/subcommands/` | `cmd/process-agent/subcommands/subcommands.go` |
| Security Agent | `cmd/security-agent/command` | `cmd/security-agent/subcommands/` | `cmd/security-agent/subcommands/subcommands.go` |
| System Probe | `cmd/system-probe/command` | `cmd/system-probe/subcommands/` | `cmd/system-probe/subcommands/subcommands.go` |
| DogStatsD | `cmd/dogstatsd/command` | `cmd/dogstatsd/subcommands/` | `cmd/dogstatsd/subcommands/subcommands.go` |

Each agent defines a `GlobalParams` struct and a `SubcommandFactory` type in its command package.

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides values, skip those questions.

1. **Target agent**: Which agent binary should this subcommand be added to? (Core Agent is most common)

2. **Subcommand name**: The command name (e.g. `health`, `hostname`, `flare`).

3. **Description**: What does the subcommand do? (used for `Short` in cobra.Command)

4. **Complexity level**: This determines the pattern to use:
   - **Simple** — No Fx components needed. Pure logic, prints output, exits. (e.g. `version`)
   - **With config/IPC** — Needs agent configuration and/or to query the running agent via IPC. Uses `fxutil.OneShot` with `core.Bundle()`. (e.g. `hostname`, `health`)

5. **Shared across agents?**: Should the implementation be shared with other agent binaries?
   - **Yes** — Create shared implementation in `pkg/cli/subcommands/<name>/`
   - **No** — Implement directly in `cmd/<agent>/subcommands/<name>/`

6. **Flags**: Does the subcommand need any CLI flags? If so, what are their names, types, defaults, and descriptions?

### Step 2: Create the subcommand package

Create the directory and `command.go` file.

#### Pattern A: Simple command (no Fx)

For commands that don't need config, logging, or IPC.

**File:** `cmd/<agent>/subcommands/<name>/command.go`

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package <name>

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
)

// Commands returns the subcommand.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "<name>",
		Short: "<description>",
		Long:  `<longer description>`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Implementation here
			fmt.Println("output")
			return nil
		},
	}
	return []*cobra.Command{cmd}
}
```

#### Pattern B: Command with Fx components (config/IPC)

For commands that need to load config, use logging, or query the running agent.

**File:** `cmd/<agent>/subcommands/<name>/command.go`

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package <name>

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	*command.GlobalParams
	logLevelDefaultOff command.LogLevelDefaultOff
	// Add custom flags here
}

// Commands returns the subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	cmd := &cobra.Command{
		Use:   "<name>",
		Short: "<description>",
		Long:  `<longer description>`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath,
						config.WithExtraConfFiles(globalParams.ExtraConfFilePath),
						config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams: log.ForOneShot(command.LoggerName, cliParams.logLevelDefaultOff.Value(), true),
				}),
				core.Bundle(),
				secretsnoopfx.Module(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	cliParams.logLevelDefaultOff.Register(cmd)
	// Add custom flags:
	// cmd.Flags().BoolVarP(&cliParams.myFlag, "flag-name", "f", false, "flag description")

	return []*cobra.Command{cmd}
}

func run(_ log.Component, _ config.Component, cliParams *cliParams, client ipc.HTTPClient) error {
	// Implementation here
	// Use client.NewIPCEndpoint("/agent/<endpoint>") to query the running agent
	fmt.Println("output")
	return nil
}
```

**IPC module variants** (pick the right one):
- `ipcfx.ModuleReadOnly()` — Read-only IPC client (most common for subcommands)
- `ipcfx.ModuleInsecure()` — Insecure IPC (for commands not needing auth)
- `ipcfx.Module()` — Full secure IPC client

#### Pattern C: Shared implementation

If the command is shared across multiple agents, create the core logic in `pkg/cli/subcommands/<name>/command.go` with its own `GlobalParams` struct, then create thin wrappers per agent.

**Shared impl:** `pkg/cli/subcommands/<name>/command.go`
```go
package <name>

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// GlobalParams contains the values of agent-level configuration passed in by each agent binary.
type GlobalParams struct {
	ConfFilePath string
	ConfigName   string
	LoggerName   string
}

// MakeCommand returns a *cobra.Command for this subcommand.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "<name>",
		Short: "<description>",
		RunE: func(_ *cobra.Command, _ []string) error {
			globalParams := globalParamsGetter()
			return fxutil.OneShot(run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath,
						config.WithConfigName(globalParams.ConfigName)),
					LogParams: log.ForOneShot(globalParams.LoggerName, "off", true),
				}),
				core.Bundle(),
			)
		},
	}
	return cmd
}

func run(_ log.Component, _ config.Component) error {
	// Shared implementation
	return nil
}
```

**Agent wrapper:** `cmd/agent/subcommands/<name>/command.go`
```go
package <name>

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/<name>"
)

// Commands returns the subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := <name>.MakeCommand(func() <name>.GlobalParams {
		return <name>.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
			ConfigName:   command.ConfigName,
			LoggerName:   command.LoggerName,
		}
	})
	return []*cobra.Command{cmd}
}
```

### Step 3: Register the subcommand

Edit the agent's `subcommands.go` file to import and register the new command.

1. Add an import with the `cmd<Name>` alias convention:
   ```go
   cmd<Name> "github.com/DataDog/datadog-agent/cmd/<agent>/subcommands/<name>"
   ```

2. Add `cmd<Name>.Commands` to the factory slice returned by the registration function (e.g. `AgentSubcommands()`).

### Step 4: Add flags (if needed)

Add flags in the `Commands` function before returning:

```go
// Boolean flag
cmd.Flags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "Enable verbose output")

// String flag
cmd.Flags().StringVarP(&cliParams.output, "output", "o", "", "Output file path")

// Int flag
cmd.Flags().IntVarP(&cliParams.timeout, "timeout", "t", 30, "Timeout in seconds")

// Persistent flag (inherited by subcommands)
cmd.PersistentFlags().BoolVarP(&cliParams.json, "json", "j", false, "Output as JSON")
```

For log level override (common pattern), use:
```go
cliParams.logLevelDefaultOff.Register(cmd)
```

### Step 5: Verify

1. Build:
   ```bash
   dda inv agent.build --build-exclude=systemd
   ```

2. Test the command:
   ```bash
   ./bin/agent/agent <name> --help
   ```

3. Run the linter:
   ```bash
   dda inv linter.go
   ```

4. Report the results to the user.

## Important Notes

- The `fxutil.OneShot` pattern runs the Fx dependency injection container, calls the handler function, then shuts down. It's the standard pattern for CLI commands that do one thing and exit.
- The handler function's parameters are injected by Fx — declare what you need (log, config, IPC client, etc.) and Fx provides them.
- `command.LogLevelDefaultOff` is a helper that adds a `--log_level` flag defaulting to `"off"` — useful for subcommands that shouldn't produce log output by default.
- Each agent has slightly different `GlobalParams` fields. Always check the target agent's `command/command.go` for available fields.

## Usage

- `/create-subcommand` — Interactive: prompts for all details
- `/create-subcommand agent my-command` — Pre-fills agent and name
