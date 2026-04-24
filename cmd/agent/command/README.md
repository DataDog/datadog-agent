# cmd/agent/command

This package builds the root cobra command for the `agent` binary, including
all subcommands. It also handles **remote commands**: CLI commands that are
defined by remote agents rather than compiled into the binary.

## Remote commands

Remote agents (trace-agent, process-agent, OTel collector, etc.) can expose CLI
commands through the `RemoteCommandProvider` gRPC service. When the Core Agent
is running and remote agents are registered, their commands are fetched at CLI
startup and registered as native cobra subcommands alongside the built-in ones.

### How it works

`MakeCommand` in `command.go` builds the command tree in two phases:

1. **Static subcommands** are added from the `SubcommandFactory` list
   (`subcommands.go`). These are compiled into the binary.

2. **Remote commands** are fetched from the running Core Agent via a lightweight
   gRPC call (`fetchRemoteCommands` in `remote.go`). For each remote command,
   a cobra command is built with proper flags, help text, and children derived
   from the proto `Command` definition. These are then added to the root
   command.

Because remote commands are registered as real cobra subcommands, they
automatically appear in `agent --help`, support `--help` on individual
commands, get flag parsing and validation from cobra, and participate in tab
completion.

### Aliases (overriding local subcommands)

A remote command can declare an `alias` in its `Command` proto definition. When
an alias matches the name of an existing local subcommand, the local subcommand
is removed from the cobra tree and replaced by the remote command. This allows
transparent migration of functionality from the Core Agent to a remote agent:
users continue running `agent <command>` and the behavior changes without any
command-line differences.

When the remote agent disconnects, the alias is no longer present at the next
CLI invocation, and the built-in local subcommand takes over again.

### Startup cost

The remote command fetch uses a lightweight config bootstrap (no Fx container)
that reads the default `datadog.yaml`, loads the auth token and IPC certificate
from disk, and makes a single gRPC call to `ListRemoteCommands` on the Core
Agent. The entire operation has an aggressive timeout
(`fetchRemoteCommandsTimeout`), so when the agent is not running the cost is
negligible: config read fails or the connection times out, and `MakeCommand`
proceeds with only the static subcommands.

### Execution

Each generated cobra command's `RunE` bootstraps a full Fx container (config +
IPC, same as any other one-shot subcommand) and sends an `ExecuteRemoteCommand`
gRPC request to the Core Agent. The Core Agent looks up which remote agent owns
the command and proxies the request. The response's stdout/stderr are printed
and the exit code is forwarded.

### Discoverability

The `agent remote list` subcommand (in `cmd/agent/subcommands/remotecommand`)
provides an explicit way to see all remote commands grouped by the agent that
provides them, independent of whether they were successfully registered at
startup.

### Key files

| File | Purpose |
|------|---------|
| `command.go` | Root cobra command, static subcommand registration, remote command integration in `MakeCommand` |
| `remote.go` | `fetchRemoteCommands` (lightweight IPC bootstrap), `buildCobraCommand` (proto-to-cobra conversion), flag registration and type mapping |
| `../subcommands/remotecommand/command.go` | `agent remote list` subcommand |

### Proto definitions

Remote command definitions live in `pkg/proto/datadog/remoteagent/command.proto`:

- `Command` — name, help text, parameters, children, alias
- `CommandParameter` — name, type, required, is_flag, is_persistent
- `RemoteCommandProvider` — gRPC service with `ListCommands` and `ExecuteCommand`
- `ListAllRemoteCommandsResponse` — commands grouped by agent (used by the Core Agent proxy)
