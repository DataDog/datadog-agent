# Overview of Fx

The Agent uses [Fx](https://uber-go.github.io/fx) to assemble applications through dependency injection. Component implementations receive ordinary Go values, while wrapper packages describe how the application obtains those values. This separation lets the same implementation run inside an Agent binary or be constructed directly by tests and other callers.

The [creation tutorial](../../tutorials/components/creating-components.md) demonstrates this separation with a compression component. The [component guidelines](../../guidelines/components.md) define the repository conventions.

## Providing and requiring

Fx connects providers and consumers by type. A constructor makes its outputs available to the application, and another constructor can request those types as dependencies. Registering a provider records how to construct a value; construction happens only when the application needs that value. Fx shares the constructed value with its consumers within the application.

For example, the tutorial's compression implementation requires configuration and logging. When an application requests the compression interface, Fx first resolves those dependencies and then calls the compression constructor. Unused providers remain unconstructed.

A component can become required through another constructor's dependencies or through an application entry point:

* [`fx.Invoke`](https://pkg.go.dev/go.uber.org/fx#Invoke) requests its arguments and runs during application construction. It can request several dependencies and return an error.
* [`fx.Populate`](https://pkg.go.dev/go.uber.org/fx#Populate) requests values and stores them in the variables addressed by its pointer arguments. Tests can then inspect or call the assembled components.

These alternative fragments show the same request. Here, `compression` is the tutorial's interface package and `fx` is `go.uber.org/fx`; each option is applied to an application that registers the compression wrapper and its dependencies.

```go
var comp compression.Component
fx.Populate(&comp)
```

```go
var comp compression.Component
fx.Invoke(func(value compression.Component) {
    comp = value
})
```

Underlying Fx also supports [`fx.Provide`](https://pkg.go.dev/go.uber.org/fx#Provide) for ordinary provider functions and [`fx.Supply`](https://pkg.go.dev/go.uber.org/fx#Supply) for existing values. A supplied value is registered under its concrete type, so it does not automatically satisfy every interface that type implements. A provider with an interface return type exposes that interface explicitly.

## Apps and options

Registration helpers return options describing an application. [`fx.New`](https://pkg.go.dev/go.uber.org/fx#New) applies those options, resolves dependencies requested by invocations and population, and reports construction errors through `app.Err()`. A missing dependency, duplicate provider, or constructor error prevents startup. `app.Run()` starts the application, waits for a shutdown signal, and stops it.

[`fx.Options`](https://pkg.go.dev/go.uber.org/fx#Options) combines options, while [`fx.Module`](https://pkg.go.dev/go.uber.org/fx#Module) also groups them under a name and scopes operations such as `fx.Decorate` and `fx.Replace`. Agent wrappers return `fxutil.Module` through `fxutil.Component(...)`, which derives the module's name from its location. This Agent wrapper type differs from the `fx.Option` returned directly by `fx.Module(...)`.

Agent binaries commonly use `fxutil.Run` for a long-running application or `fxutil.OneShot` for a command. Both include the Agent's lifecycle and shutdown adapters and Fx logging. `OneShot` calls its command function after startup completes and shuts the application down afterward. Ordinary `fx.Invoke` callbacks run earlier, during construction.

## Lifecycle

Construction establishes dependencies; lifecycle hooks coordinate work that must start and stop with the application. Implementations express these hooks through <<<repo("comp/def/lifecycle.go", "`compdef.Lifecycle` and `compdef.Hook`")>>> so their code remains independent of Fx.

An implementation receives a lifecycle in its `Requires` struct and appends hooks during construction. The application goes through four phases:

1. Initialization constructs requested components and runs invocations. Hooks are registered but have not run.
1. Startup calls `OnStart` hooks in registration order. Dependencies are constructed before their consumers, so hooks appended by those constructors follow that order.
1. Runtime begins after startup completes. Startup hooks schedule ongoing work and return, allowing later hooks to run.
1. Shutdown calls `OnStop` hooks in reverse order. If startup fails, Fx rolls back the hooks whose startup completed successfully.

An unused provider never gets an opportunity to register its hooks. This is why registering a component's wrapper alone does not start its background work. Public methods may also be called by an invocation before startup, which motivates the [component lifecycle requirements](../../guidelines/components.md#concurrency-and-lifecycle).

The boundary between the plain lifecycle and Fx is explicit. <<<repo("pkg/util/fxutil/provide_comp.go", "`fxutil.FxLifecycleAdapter()`", match="^func FxLifecycleAdapter")>>> supplies `compdef.Lifecycle` from Fx's lifecycle. `fxutil.FxAgentBase()` includes that adapter, the shutdown adapter, and Fx logging. An application built directly with `fx.New` needs the appropriate option when its components request these interfaces. The constructor adapter does not supply lifecycle adaptation.

`fxutil.Run` and `fxutil.OneShot` already register `FxAgentBase`. Adding either adapter option to them, or combining both options in an `fx.New` application, duplicates providers.

Direct callers provide their own lifecycle implementation. Tests can use `compdef.NewTestLifecycle(t)` with the repository's `test` build tag. Older Agent implementations and underlying Fx examples may instead depend directly on `fx.Lifecycle` and `fx.Hook`.

Fx applies startup and shutdown timeouts when running an application. Hooks that respect cancellation of their supplied context allow those limits to take effect.

## Ins and outs

[`fx.In`](https://pkg.go.dev/go.uber.org/fx#In) and [`fx.Out`](https://pkg.go.dev/go.uber.org/fx#Out) identify dependency and result structs to Fx. Agent implementations use plain exported `Requires` and `Provides` structs so that their constructors also work without Fx.

<<<repo("pkg/util/fxutil/provide_comp.go", "`fxutil.ProvideComponentConstructor`")>>> creates an adapter around the plain constructor. It adds Fx's markers to generated input and output types, preserves exported fields and their tags, and translates values between the generated types and the implementation's structs. A `Provides` struct can expose several types; a constructor can also return an error, which the adapter passes through to Fx.

For example, the compression tutorial's output and the adapter's corresponding output have these conceptual shapes. The `compression` alias denotes the tutorial's interface package and `fx` denotes `go.uber.org/fx`. The generated type is shown only to explain the adapter.

```go
type Provides struct {
    Comp compression.Component
}

type fxProvides struct {
    fx.Out
    Comp compression.Component
}
```

The adapter rejects unexported fields and manually included `fx.In` or `fx.Out` fields. The wrapper owns Fx registration, while the implementation's types describe its dependencies and results. The [constructor conventions](../../guidelines/components.md#package-and-constructor-conventions) capture this boundary.

### Value groups

[Value groups](https://pkg.go.dev/go.uber.org/fx#hdr-Value_Groups) let several providers contribute values of the same type. A producer tags an output with a group name, and a consumer requests a slice with that name. The constructor adapter preserves these tags.

The <<<repo("comp/core/remoteagentregistry/impl/registry.go", "remote-agent registry")>>> uses this mechanism for event subscribers. Its <<<repo("comp/core/remoteagentregistry/def/subscriber.go", "subscriber helper")>>> returns an Fx-aware struct embedding `fx.Out`. The following example adapts the subscriber type and group tag to plain producer and consumer structs registered through `fxutil.ProvideComponentConstructor`. Only the group-related fields are shown; `remoteagentregistry` denotes <<<repo("comp/core/remoteagentregistry/def", "the registry's interface package")>>>.

```go
type Provides struct {
    Subscriber *remoteagentregistry.EventSubscriber `group:"remoteAgentEventSubscriber"`
}

type Requires struct {
    EventSubscribers []*remoteagentregistry.EventSubscriber `group:"remoteAgentEventSubscriber"`
}
```

When the registry is requested, Fx constructs the providers contributing subscribers. Their order in the resulting slice is unspecified. A group with no providers yields an empty slice, so the registry can accept any number of subscribers.

### Optional values

An `option.Option[T]` dependency is an ordinary required type to Fx. It needs a provider even when its value represents absence. This differs from a value group, which can resolve without contributors.

<<<repo("pkg/util/fxutil/provide_optional.go", "`fxutil.ProvideOptional[T]()`")>>> requires a provider for `T` and wraps its value with `option.New[T](value)`; supplying absence requires a separate provider returning `option.None[T]()`. A no-op implementation still supplies `T`; converting it to an option produces a present value.

The <<<repo("comp/core/ipc/fx-none/fx.go", "IPC `fx-none` wrapper")>>> supplies a no-op component and a present option. The <<<repo("comp/anomalydetection/recorder/fx-noop/fx_noop.go", "recorder `fx-noop` wrapper")>>> supplies an absent option. Existing package names do not consistently distinguish these behaviors.

## Related documentation

- Follow the [creation tutorial](../../tutorials/components/creating-components.md) to build and exercise a complete component.
- Follow the [optional-dependency guide](../../how-to/components/optional-dependencies.md) to make an existing consumer work with an absent dependency.
- See [using components in a binary](../../how-to/components/using-components.md) for application assembly.
- Consult the [component guidelines](../../guidelines/components.md) for repository conventions.
