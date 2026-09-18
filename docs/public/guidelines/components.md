# Component guidelines

These conventions apply to every Agent component. See the [component framework overview](../architecture/components/index.md) for the design rationale and the [creation tutorial](../tutorials/components/creating-components.md) for a worked example.

## Ownership and interfaces

- Each component must be owned by one team responsible for its support and maintenance.
- Hide implementation details behind a small public interface named `Component`, and keep concrete implementations private.
- Make components reusable across binaries and external consumers. Implementation changes that preserve the interface and its behavior should not require QA of consumers.
- Avoid exposing third-party types in public interfaces unless they are intrinsic to the component's purpose. Docker types can make sense in a Docker component, but exposing them through a generic container component forces every consumer to depend on Docker.
- Test each component's behavior through its interface.

## File hierarchy

All components are located in the `comp` folder at the top of the Agent repo.

The file hierarchy is as follows:

```
comp /
  <bundle name> /        <-- Optional
    <comp name> /
      def /              <-- The folder containing the component interface and ALL its public types.
      impl /             <-- The only or primary implementation of the component.
      impl-<alternate> / <-- An alternate implementation.
      impl-none /        <-- Optional. A noop implementation.
      fx /               <-- All fx related logic for the primary implementation, if any.
      fx-<alternate> /   <-- All fx related logic for a specific implementation.
      mock /             <-- The mock implementation of the component to ease testing.
```

To note:

* If your component has only one implementation, it should live in the `impl` folder.
* If your component has several implementations instead of a single implementation, you have multiple `impl-<version>` folders instead of an `impl` folder. For example, a compression component can have `impl-zstd` and `impl-zip` folders instead of an `impl` folder.
* If your component needs to offer a dummy/empty version, it should live in the `impl-none` folder.

## Package and constructor conventions

- Keep the interface and all public types in `def`. Consumers should not need to import implementation packages.
- Name implementation packages `<COMPONENT_NAME>impl`, or `<IMPL_NAME>impl` for an alternate implementation, and expose a `NewComponent` constructor.
- Declare constructor dependencies in a public `Requires` struct and outputs in a public `Provides` struct. Keep their fields public so the Fx wrapper can use them.
- Return `Provides` from an infallible constructor, or `(Provides, error)` when construction can fail.
- Confine Fx imports and references to the `fx` packages. Each `fx.go` must expose `func Module() fxutil.Module` and wrap the corresponding implementation.
- Supply a `fx-none` wrapper when consumers need an absent optional implementation. See [optional dependencies](../how-to/components/optional-dependencies.md).

## Mocks

Components must provide a mock implementation unless their public interface has no methods. A mock must implement the interface in `def` and may expose additional testing helpers. Mock constructors must accept a `*testing.T` parameter.

## Go modules

Go modules are optional. To make a component available outside this repository, create separate modules for its `def` package, the implementations being exported, and its mock package. Never put a Go module at the component root or in an Fx wrapper package.

See [adding nested modules](../how-to/go/modules.md) for the procedure.

## Concurrency and lifecycle

Components must be thread safe, tested, and documented. Public methods must be usable as soon as construction completes, although they may do nothing or drop data before the Agent finishes initialization. Document these behaviors and use [lifecycle hooks](../architecture/components/fx.md#lifecycle) for startup and shutdown work.

## Documentation

The documentation (both package-level and method-level) should include everything a user of the component needs to know. In particular, the documentation must address any assumptions that might lead to panic if violated by the user.

Detailed documentation of how to avoid bugs in using a component is an indicator of excessive complexity and should be treated as a bug. Simplifying the usage will improve the robustness of the Agent.

Documentation should include:

* Precise information on when each method may be called. Can methods be called concurrently?
* Precise information about data ownership of passed values and returned values. Users can assume that any mutable value returned by a component will not be modified by the user or the component after it is returned. Similarly, any mutable value passed to a component will not be later modified, whether by the component or the caller. Any deviation from these defaults should be documented.

    /// note
    It can be surprisingly hard to avoid mutating data -- for example, `append(..)` surprisingly mutates its first argument. It is also hard to detect these bugs, as they are often intermittent, cause silent data corruption, or introduce rare data races. Where performance is not an issue, prefer to copy mutable input and outputs to avoid any potential bugs.
    ///

* Precise information about goroutines and blocking. Users can assume that methods do not block indefinitely, so blocking methods should be documented as such. Methods that invoke callbacks should be clear about how the callback is invoked, and what it might do. For example, document whether the callback can block, and whether it might be called concurrently with other code.
* Precise information about channels. Is the channel buffered? What happens if the channel is not read from quickly enough, or if reading stops? Can the channel be closed by the sender, and if so, what does that mean?
