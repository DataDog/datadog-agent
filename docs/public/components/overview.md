# Overview of Components

The Agent is structured as a collection of components working together. Depending on how the binary is built, and how it
is invoked, different components may be instantiated.

<!-- TODO: What are the goals of using the components?  -->

## What is a Component?

The goal of a component is to encapsulates a particular piece of logic/feature and provide a clear and documented interface. 

A component must:

  + Hide the complexity of the implementation from its users.
  + Be reusable no matter the context or the binary in which they're included.
  + Be tested.
  + Expose a mock implementation to help testing.
  + Be owned by a single team which supports and maintains it.
  
Any change within a component that don't change its interface should not require QA of another component using it.

Since each component is an interface to the outside we can have several implementations for it.

## FX vs Go module

Components are designed to be used with a dependency injection framework. In the Agent, we use [Fx](fx.md) for this. All Agent
binaries use `FX` to load, coordinate and start the required components.

Some components are used outside the `datadog-agent` repository where `FX` is not available. To support this, the components implementation must not require `FX`. 
Component implementations can be exported as Go modules. The next section explains in more detail how to create components.

The important information here is that it's possible to use components without FX **outside** the agent repository. This
comes at the cost of manually doing the work of `Fx`, our dependency injection framework.

## Important note on Fx

The component framework project core goal is to improve the Agent codebase by decoupling parts of the code, removing global state and init
methods, and increased reusability by separating logical units into components. **`Fx` itself is not intrinsic to the
benefits of componentization**.

<!-- TODO: Let's have a disclaimer about components not being 1 to 1 to Fx -->

## Next

Next, let's see how to create a Bundle, a Component and use FX: [here](creating-components.md).
