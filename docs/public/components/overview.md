# Overview of Components

The Agent is structured as a collection of components working together. Depending on how the binary is built, and how it
is invoked, different components may be instantiated.

## What is a Component?

A component encapsulates a particular piece of logic/feature in a clear and documented interface. 

A component must:

  + The component hides the complexity of the implementation from its users.
  + All components must be reusable no matter the context or the binary in which they're included.
  + A component must be tested.
  + Expose a mock implementation to help testing.
  + Any change within a component, that don't change its interface, should not require QA of the other component using
    it.
  + Each component should be owned by a single team which will support and maintain it.

Since each component is an interface to the outside we can have several implementations for it.

## FX vs Go module

Components are designed to be used with a dependency injection framework. In the Agent we use [Fx](fx.md) for this. All agent
binaries use `FX` to load, coordinate and start the required components.

Some components are used outside the `datadog-agent` repository where `FX` is not available. To support this, the components implementation must not require `FX`. 
Components implementation can be exported as a Go module. We will see in more detail how to create components in the next section.

The important information here is that it's possible to use components without FX **outside** the agent repository. This
comes at the cost of manually doing the work of `Fx`, our dependency injection framework.

## Important note on Fx

The component framework project core goal is to improve the Agent codebase by decoupling parts of the code, removing global state and init
methods, and increased reusability by separating logical units into components. **`Fx` itself is not intrinsic to the
benefits of componentization**.

## Next

Next, let's see how to create a Bundle, a Component and use FX: [here](creating-components.md).
