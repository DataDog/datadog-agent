# Component framework

The Agent is structured as a collection of components working together. Different components may be instantiated depending on how a binary is built and invoked.

## Learning path

This collection brings the framework's concepts, tutorials, task guides, conventions, and reference together. The examples assume familiarity with Go; use [Getting started](../../../start.md) if you have not set up a development environment.

1. Read this overview, then the [Fx overview](concept/fx.md), to understand component boundaries, dependency injection, and lifecycle.
1. Read the [component guidelines](guideline.md) for the required package structure, ownership, and conventions.
1. Follow the [creation tutorial](tutorial/create.md), then the [testing tutorial](tutorial/test.md).
1. Learn how to [use components in binaries](how-to/use.md).

For a specific task or interface, use the [related documentation](#related-documentation) below or this topic's sidebar.

## What is a component?

A component encapsulates a feature behind a documented interface. Its implementation is hidden from callers, allowing the same interface to have several implementations and the same component to be reused across binaries.

Componentization decouples parts of the codebase, removes global state and initialization functions, and makes logical units reusable. These benefits come from the component boundaries themselves.

## Fx and Go modules

Agent binaries use [Fx](concept/fx.md) to load, coordinate, and start components through dependency injection. Component implementations are independent of Fx so that they can also be used outside the Agent repository.

Definitions and implementations can be exported as Go modules. A consumer that does not use Fx must construct dependencies and manage their lifecycle itself.

## Package separation

Each component separates its public interface, implementations, Fx wrappers, and mocks into packages. The [component file hierarchy](guideline.md#file-hierarchy) describes their locations.

Callers import the interface without importing an implementation. A binary chooses an implementation through its Fx wrapper, so choosing ZIP compression does not also require compiling the ZSTD library. Separate packages and Go modules also let external consumers import only the definitions and implementations they need.

This separation supports binaries with different combinations of components and consumers outside the Agent repository.

## Bundles

A bundle groups related components that work together to provide a product or feature. For example, DogStatsD consists of multiple components that a binary can include through a single bundle.

A bundle supplies a default collection of components. Binaries can also select individual components, and Fx instantiates only those that are required by the application and its dependencies.

## Related documentation

- Learn how dependency injection, application lifecycle, and value groups work in the [Fx overview](concept/fx.md).
- Follow the [component creation](tutorial/create.md) and [testing](tutorial/test.md) tutorials.
- Follow the [component guidelines](guideline.md) for ownership, package boundaries, concurrency, and documentation.
- Learn how to [use components in binaries](how-to/use.md), [create bundles](how-to/bundle.md), and [use optional dependencies](how-to/optional-dependency.md).
- Integrate components with the Agent by [adding flare data](how-to/flare.md) and [adding a status provider](how-to/status.md).
- Consult the [status provider reference](reference/status.md) for interfaces and rendering helpers.
