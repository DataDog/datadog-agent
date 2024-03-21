# Overview of Components

The Agent is structured as a collection of components working together. Depending on how the binary is built, and how it
is invoked, different components may be instantiated.

## What is a Component?

Any well-defined portion of the codebase, with a clearly documented API, _should_ be a component. The ultimate goal of
component is to clearly encapsulate the logic/feature block that composed the Datadog Agent. Components can be small or very
large but all share the same goals.

A component must:

* **Limit blast radius**:
    + Have a single interface clearly highlighting how the component must be used and what features it offers.
    + Encapsulate and abstract all the complexity of its subject. As much as possible a user of a component should not
      have to know all the internals of this one. As well, any change to the internals of a component should have no
      impact on the code using it.
    + Clearly list its dependencies, which could be passed using dependency injection (more on this below).
* **Increase development velocity**:
    + All components must be reusable no matter the context or the binary in which they're included.
    + Expose a mock implementing their interface to ease testing.
    + As long as its dependencies are present a component should work the same no matter in which context it's used.
      This means we should be able to easily create new binary/agents by composing different components.
    + The interface must be carefully documented allowing developers from other team to easily reuse it.
    + Any change within a component, that don't change its interface, should not require QA of the other component using
      it.
* **Clear ownership and responsibility**:
    + Each component should be owned by a single team which will support and maintain it.

Since each component is an interface to the outside we can have several implementations for it.

## Grouping components: Bundles

There is a large and growing number of components, and listing those components out repeatedly could grow tiresome and
cause bugs. Component bundles provide a way to manipulate multiple components, usually at the meta or service level, as
a single unit. For example, while Logs-Agent is internally composed of many components, those can be used as a single unit
with a `logs` bundle.

As an aid to thinking about this question, consider the following levels:

1. Large-scale parts of the Agent that agglomerate multiple components would be a `Bundle`. Example: the Logs-Agent.
2. Dedicated section or logic block that can be reused in multiple places would be a `Component`. Example: the logger
   component.
3. A specific version of a component would be an implementation. Example: a component in charge of compressing payload
   could have a ZSTD and ZIP implementation. Most components will only have a single implementation.

## Stateless logic

Any logic/helper that is stateless (no globals and not `init` function) can but doesn't need to be a component. Such
basic helpers can remain or be added to the `pkg` folder.

## FX vs Go module

Components are designed to be used with a dependency injection. In the Agent repo we use [Fx](fx.md) for this. All agent
binaries use `FX` to load, coordinate and start the required components.

Some components are used outside the `datadog-agent` repository where `FX` is not available. To support this, the
`FX` wrappers are split from each component's implementations. Components implementation can be exported as a Go module. We
will see in more detail how to create components in the next section.

The important information here is that it's possible to use components without FX **outside** the agent repository. This
comes at the cost of manually doing the work of `Fx`, our dependency injection framework.

## Next

Next, let's see how to create a Bundle, a Component and use FX: [here](creating-components.md).
