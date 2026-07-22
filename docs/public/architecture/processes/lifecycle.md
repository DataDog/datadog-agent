# Startup and lifecycle

-----

Every Agent binary boots the same way: a cobra command tree parses the CLI, the selected subcommand assembles an Fx dependency-injection app from component modules, Fx constructs the components that are actually needed, runs their start hooks, and hands control to the binary's main function. Shutdown reverses the hooks. This page walks that path — `fxutil.OneShot` versus `fxutil.Run`, hook ordering, timeouts, and the several ways an agent process exits. The component framework itself (def/impl/fx layout, bundles, value groups) is documented in the [component overview](../components/overview.md) and the [Fx primer](../components/fx.md); this page is about how a *process* uses it.

## Key packages and files

| Path | Purpose |
|---|---|
| [`pkg/util/fxutil/oneshot.go`](<<<SRC>>>/pkg/util/fxutil/oneshot.go) | `OneShot(fn, opts...)`: start app, call `fn` after all OnStart hooks, stop |
| [`pkg/util/fxutil/run.go`](<<<SRC>>>/pkg/util/fxutil/run.go) | `Run(opts...)`: daemon mode, blocks on `app.Done()` |
| [`pkg/util/fxutil/args.go`](<<<SRC>>>/pkg/util/fxutil/args.go) | `delayedFxInvocation`: captures the callback's injected arguments during startup |
| [`pkg/util/fxutil/provide_comp.go`](<<<SRC>>>/pkg/util/fxutil/provide_comp.go) | `FxAgentBase()`, lifecycle/shutdowner adapters, the reflection bridge |
| [`pkg/util/fxutil/timeout.go`](<<<SRC>>>/pkg/util/fxutil/timeout.go) | `TemporaryAppTimeouts()`: 5-minute default start/stop timeouts |
| [`pkg/util/fxutil/errorunwrapper.go`](<<<SRC>>>/pkg/util/fxutil/errorunwrapper.go) | Unwraps Fx-mangled constructor errors |
| [`pkg/util/fxutil/logging`](<<<SRC>>>/pkg/util/fxutil/logging) | Fx event logging (`TRACE_FX`) and startup-span tracing |
| [`comp/def/lifecycle.go`](<<<SRC>>>/comp/def/lifecycle.go) | `compdef.Lifecycle`: the fx-free OnStart/OnStop hook API components use |
| [`comp/def/shutdowner.go`](<<<SRC>>>/comp/def/shutdowner.go) | `compdef.Shutdowner`: lets a component trigger app shutdown |
| [`cmd/agent/subcommands/run/command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go) | The canonical composition root: `getSharedFxOption()`, `run(...)`, `startAgent`, `stopAgent` |
| [`cmd/agent/common/signals/signals.go`](<<<SRC>>>/cmd/agent/common/signals/signals.go) | `signals.Stopper` / `signals.ErrorStopper` channels used to stop the core agent |
| [`pkg/util/winutil/servicemain/servicemain.go`](<<<SRC>>>/pkg/util/winutil/servicemain/servicemain.go) | Windows service wrapper around the same boot path |

## From `main()` to an Fx app

Every binary follows the cobra + fxutil pattern described in [using components](../components/using-components.md): `cmd/<binary>/main.go` builds a root command, each subcommand's `RunE` calls either `fxutil.OneShot(callback, opts...)` or `fxutil.Run(opts...)`, and `opts` is a list of `fx.Supply` (params structs), component `Module()`s, and `Bundle()`s. For the core agent the closest thing to a composition root is `getSharedFxOption()` in [`cmd/agent/subcommands/run/command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go): the core bundle with secrets enabled, the forwarder, workloadmeta, the dual tagger, autodiscovery, DogStatsD, logs, metadata, remote-config and other bundles, plus dozens of individual modules and `fx.Provide` shims for not-yet-componentized types. Per-OS composition is isolated in `getPlatformModules()` ([`command_windows.go`](<<<SRC>>>/cmd/agent/subcommands/run/command_windows.go) / [`command_notwin.go`](<<<SRC>>>/cmd/agent/subcommands/run/command_notwin.go)).

Two properties of Fx shape everything downstream:

1. **Instantiation is lazy.** Importing a bundle only teaches Fx how to build things. A component is constructed only if the callback (or an `fx.Invoke`) requires it, directly or transitively. This is why the core agent's `run(...)` function takes ~45 injected arguments, many named `_` — they exist solely to force those components into existence. The equivalent idiom elsewhere is `fx.Invoke(func(_ x.Component) {})`, as in [`command_snmptraps.go`](<<<SRC>>>/cmd/agent/subcommands/run/command_snmptraps.go).
1. **Hook ordering follows the dependency graph.** OnStart hooks run in construction order, OnStop hooks in reverse. There is no way to order hooks between unrelated components except by declaring a dependency; `command.go` contains documented `fx.Invoke` hacks that list components as parameters purely to sequence hook registration.

Both `OneShot` and `Run` always append `FxAgentBase()` (which provides the `compdef.Lifecycle` and `compdef.Shutdowner` adapters plus the Fx event logger) and *prepend* `TemporaryAppTimeouts()`, so callers can override the timeouts with their own options.

## OneShot: CLI commands (and, surprisingly, `agent run`)

`fxutil.OneShot` ([`oneshot.go`](<<<SRC>>>/pkg/util/fxutil/oneshot.go)) exists because Fx has no "run this invoke last" primitive. It wraps the callback in a `delayedFxInvocation` ([`args.go`](<<<SRC>>>/pkg/util/fxutil/args.go)): an `fx.Invoke`d shim with the same signature that only *captures* the injected arguments while the app starts. The sequence is:

1. `fx.New(opts...)` — constructors run lazily, in dependency order, as required types are resolved.
1. `app.Start(ctx)` — all OnStart hooks run under the (shared) start timeout. A failure here aborts and still runs `app.Stop`.
1. `delayedCall.call()` — the real callback runs with the captured arguments.
1. When the callback returns, `app.Stop(ctx)` runs the OnStop hooks in reverse order.

Every CLI subcommand (`agent status`, `agent flare`, ...) is a OneShot. So is `agent run` itself: the daemon behavior is emulated by the `run(...)` callback blocking on a channel. There is an explicit TODO in `command.go` to move to `fxutil.Run` once the agent is fully represented as a component. Concretely, `run(...)`:

1. Defers `stopAgent(...)` so teardown happens no matter how the callback exits.
1. Subscribes to `os.Interrupt`/`SIGTERM`, and to the package-level channels `signals.Stopper` (used by the `agent stop` CLI command via the API server) and `signals.ErrorStopper` (used by fatal internal errors) — see [`signals.go`](<<<SRC>>>/cmd/agent/common/signals/signals.go).
1. Swallows `SIGPIPE` entirely, because systemd redirects stdout to journald and a journald crash would otherwise kill the agent.
1. Calls `startAgent(...)`, which performs the remaining imperative (non-Fx) wiring: registering checks, starting autodiscovery `LoadAndRun`, JMX, the diagnose catalog, the CLC runner server, and so on.
1. Blocks until one of the stop channels fires, then returns, letting the deferred `stopAgent` and the Fx OnStop hooks clean up.

## Run: true Fx daemons

`fxutil.Run` ([`run.go`](<<<SRC>>>/pkg/util/fxutil/run.go)) builds the app, starts it, then blocks on `<-app.Done()`, which fires on SIGINT/SIGTERM or when any component calls `Shutdowner.Shutdown()`. Binaries using it include the [trace-agent](<<<SRC>>>/cmd/trace-agent/subcommands/run/command.go), the [otel-agent](<<<SRC>>>/cmd/otel-agent/subcommands/run/command.go), the [systray](<<<SRC>>>/cmd/systray/command/command.go), the [installer daemon](<<<SRC>>>/cmd/installer/subcommands/daemon/run_nix.go), and the [private action runner](<<<SRC>>>/cmd/privateactionrunner/subcommands/run/command.go). In these processes the daemon's work lives entirely in component OnStart hooks and background goroutines; there is no blocking callback.

/// note
Do not assume `Shutdowner` semantics in the core agent — it uses the `signals` channels instead. A component that calls `Shutdowner.Shutdown()` will stop the trace-agent but not `agent run`.
///

## Lifecycle phases and the component contract

1. **Construction** — constructors run lazily during `fx.New`/`app.Start`. A constructor returning a non-nil error aborts startup.
1. **Startup** — OnStart hooks run in construction order. Components register hooks through `compdef.Lifecycle` ([`comp/def/lifecycle.go`](<<<SRC>>>/comp/def/lifecycle.go)), the fx-free mirror of `fx.Lifecycle` adapted by `FxAgentBase()`.
1. **Runtime** — OneShot: the delayed callback runs; Run: the app blocks on `Done()`.
1. **Shutdown** — OnStop hooks run in reverse order, under the stop timeout.

The framework's design rule (see [creating components](../components/creating-components.md)) is that every public method must be safe to call as soon as the constructor returns — it may no-op or drop data before start. One ordering rule worth knowing at the process level: gRPC services must be registered in constructors, not OnStart hooks, because the API server starts serving immediately after its own OnStart.

## Timeouts

`TemporaryAppTimeouts()` ([`timeout.go`](<<<SRC>>>/pkg/util/fxutil/timeout.go)) sets both the Fx start and stop timeouts to 5 minutes, overridable via `DD_FX_START_TIMEOUT_SECONDS` and `DD_FX_STOP_TIMEOUT_SECONDS` (integer seconds). Before Fx, the Agent had no start/stop timeout at all, and the value is deliberately generous.

/// warning
The service manager will usually kill the process long before Fx gives up. systemd's default stop timeout is 90 seconds, upstart's is 5, and the Windows service wrapper has its own `HardStopTimeout` in [`servicemain.go`](<<<SRC>>>/pkg/util/winutil/servicemain/servicemain.go). A slow OnStop hook therefore manifests as a SIGKILL from the supervisor, not as an Fx timeout error.
///

## Shutdown paths

An agent process can exit through several doors, all converging on the OnStop hooks:

1. **Signals**: SIGTERM/SIGINT (systemd `systemctl stop`, container runtime, Ctrl-C). OneShot daemons translate these to a callback return; Run daemons get `app.Done()`.
1. **`agent stop`**: the CLI posts to the running agent's API, whose handler writes to `signals.Stopper`.
1. **Internal fatal error**: components write to `signals.ErrorStopper`, producing a nonzero exit.
1. **`Shutdowner.Shutdown()`**: components in `fxutil.Run` binaries can stop the app programmatically.
1. **Windows service control**: the SCM stop request is translated by [`servicemain.go`](<<<SRC>>>/pkg/util/winutil/servicemain/servicemain.go), which also contains deliberate exit-code gates so config-triggered restarts do not trip the service failure actions.
1. **Auto-exit**: the `autoexit` component (part of the agent bundle) can arm an exit condition, used in containers to make the process exit when a sidecar lifetime ends.

On Linux hosts, the systemd unit tree adds one more layer: satellite units are `BindsTo=datadog-agent.service`, so stopping the core agent stops the whole family. See [Process supervision](supervision.md).

## Startup failure modes and debugging

1. **Missing type**: an `fx.Invoke` or callback parameter that nothing provides fails app construction with a dependency-graph error listing the missing type. Check that the module providing it is in the option list — remember that misspelled or absent modules fail at runtime, not compile time.
1. **Constructor error**: Fx wraps these in an unexported `errArgumentsFailed`; `UnwrapIfErrArgumentsFailed` ([`errorunwrapper.go`](<<<SRC>>>/pkg/util/fxutil/errorunwrapper.go)) regex-extracts the real message, which is why startup errors sometimes look oddly flattened.
1. **Hook timeout**: an OnStart hook exceeding the start timeout aborts startup; an OnStop hook exceeding the supervisor's kill timeout gets the process killed (see above).
1. **Silent no-op**: a component that nothing requires is never constructed. If your component "never starts", it probably was never instantiated — force it with a `_ x.Component` callback argument or an `fx.Invoke`.

Observability of the boot itself:

| Mechanism | How | What you get |
|---|---|---|
| `TRACE_FX=1` | environment variable | Every Fx event (provide/invoke/hook) logged to stderr |
| debug log level | `EnableFxLoggingOnDebug` in [`logging`](<<<SRC>>>/pkg/util/fxutil/logging) | Fx events routed into the agent log once the logger exists |
| `DD_FX_TRACING_ENABLED=true` | [`logging/tracer.go`](<<<SRC>>>/pkg/util/fxutil/logging/tracer.go) + [`fxinstrumentation`](<<<SRC>>>/comp/core/fxinstrumentation/fx/fx.go) | A span per constructor and OnStart hook, shipped to the local trace-agent as service `dd-agent-fx-init` |

## Per-binary variations

1. **Windows services** wrap the same Fx app: [`cmd/agent/main_windows.go`](<<<SRC>>>/cmd/agent/main_windows.go) routes service launches through `servicemain.Run`, which then invokes the same run subcommand internals.
1. **The standalone DogStatsD** ([`cmd/dogstatsd/subcommands/start/command.go`](<<<SRC>>>/cmd/dogstatsd/subcommands/start/command.go)) assembles a small fraction of the core agent's modules — a good minimal example of a composition root.
1. **serverless-init** ([`cmd/serverless-init/main.go`](<<<SRC>>>/cmd/serverless-init/main.go)) boots through `fxutil.OneShot` like everything else, but is built with the `serverless` build tag, which swaps several components for slimmer implementations (noop telemetry, local-only tagger, serverless trace config) to minimize cold-start time.
1. **Satellite processes** (trace-agent, process-agent, security-agent) wire remote client components (remote tagger, configsync) whose OnStart hooks connect back to the core agent — their startup can therefore block on `auth_init_timeout` waiting for the core agent's IPC artifacts; see [Inter-process communication](ipc.md).
