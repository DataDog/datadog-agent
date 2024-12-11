# Overview of components

The Agent is structured as a collection of components working together. Depending on how the binary is built, and how it
is invoked, different components may be instantiated.

<!-- TODO: What are the goals of using the components?  -->

## What is a component?

The goal of a component is to encapsulate a particular piece of logic/feature and provide a clear and documented interface. 

A component must:

  + Hide the complexity of the implementation from its users.
  + Be reusable no matter the context or the binary in which they're included.
  + Be tested.
  + Expose a mock implementation to help testing if it makes sense.
  + Be owned by a single team which supports and maintains it.
  
Any change within a component that don't change its interface should not require QA of another component using it.

Since each component is an interface to the outside, it can have several implementations.

## Fx vs Go module

Components are designed to be used with a dependency injection framework. In the Agent, we use [Fx](fx.md), a dependency injection framework, for this. All Agent
binaries use Fx to load, coordinate, and start the required components.

Some components are used outside the `datadog-agent` repository, where Fx is not available. To support this, the components implementation must not require Fx. 
Component implementations can be exported as Go modules. The next section explains in more detail how to create components.

The important information here is that it's possible to use components without Fx **outside** the Agent repository. This
comes at the cost of manually doing the work of Fx.

## Important note on Fx

The component framework project's core goal is to improve the Agent codebase by decoupling parts of the code, removing global state and init
functions, and increasing reusability by separating logical units into components. **Fx itself is not intrinsic to the
benefits of componentization**.

<!-- TODO: Let's have a disclaimer about components not being 1 to 1 to Fx -->

## Next

Next, see how to [create a bundle and a component](creating-components.md) by using Fx.
