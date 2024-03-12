# Overview of Components

The Agent is structured as a collection of components working together. Depending on how the binary is built, and how it is invoked, different components may be instantiated. The behavior of the components depends on the Agent configuration.

Components are structured in a dependency graph. For example, the `comp/logs/agent` component depends on the `comp/core/config` component to access Agent configuration. At startup, a few top-level components are requested, and [Fx](fx.md) automatically instantiates all of the required components.

## What is a Component?

Any well-defined portion of the codebase, with a clearly documented API surface, _can_ be a component. As an aid to thinking about this question, consider four "levels" where it might apply:

1. Meta: large-scale parts of the Agent that use many other components. Example: DogStatsD or Logs-Agent.
2. Service: something that can be used at several locations (for example by different applications). Example: Forwarder.
3. Internal: something that is used to implement a service or meta component, but doesn't make sense outside the component. Examples: DogStatsD's TimeSampler, or a workloadmeta Listener.
4. Implementation: a type that is used to implement internal components. Example: Forwarder's DiskUsageLimit.

In general, meta and service-level functionality should always be implemented as components. Implementation-level functionality should not. Internal functionality is left to the descretion of the implementing team: it's fine for a meta or service component to be implemented as one large, complex component, if that makes the most sense for the team.

## Bundles

There is a large and growing number of components, and listing those components out repeatedly could grow tiresome and cause bugs. Component bundles provide a way to manipulate multiple components, usually at the meta or service level, as a single unit. For example, while Logs-Agent is internally composed of many components, those can be addressed as a unit with `comp/logs.Bundle`.

Bundles also provide a way to provide parameters to components at instantiation. Parameters can control the behavior of components within a bundle at a coarse scale, such as whether the logs-agent should start or not.

## Apps and Binaries

The build infrastructure builds several agent binaries from the agent source. Some are purpose-specific, such as the serverless agent or dogstatsd, while others such as the core agent support many kinds of functionality. Each build is made from a subset of the same universe of components. For example, the components comprising the DogStatsD build are precisely the same components implementing the DogStatsD functionality in the core agent.

Most binaries support subcommands, such as `agent run` or `agent status`. Each of these also uses a subset of the components available in the binary, and perhaps some different bundle parameters. For example, `agent status` does not need the logs-agent bundle (`comp/logs.Bundle`), and does not need to start core-bundle services like component health monitoring.

These subcommands are implemented as Fx _apps_. An app specifies, the set of components that can be instantiated, their parameters, and the top-level components that should be requested.

There are utility functions available in `pkg/util/fxutil` to eliminate some common boilerplate in creating and running apps.

### Build-Time and Runtime Dependencies

Let's consider sets and subsets of components. Each of the following sets is a subset of the previous set:

1. All implemented components (everything in [this document][agent-components-listing])
1. All components in a binary (everything directly or indirectly referenced by a binary's `main()`) -- the _build-time dependencies_
1. All components available in an app (everything provided by a bundle in the app's `fx.New` call)
1. All components instantiated in an app (all explicitly required components and their transitive dependencies) -- the _runtime dependencies_
1. All components started in an app (all instantiated components, except those disabled by their parameters)

The build-time dependencies determine the binary size. For example, omitting container-related components from a binary dramatically reduces binary size by not requiring kubernetes and docker API libraries.

The runtime dependencies determine, in part, the process memory consumption. This is a small effect because many components will use only a few Kb if they are not actually doing any work. For example, if the trace-agent's trace-writer component is instantiated, but writes no traces, the peformance impact is trivial.

The started components determine CPU usage and consumption of other resources. A component polling a data source unnecessarily is wasteful of CPU resources. But perhaps more critically for correct behavior, a component started when it is not needed may open network ports or engage other resources unnecessarily. For example, `agent status` should not open a listening port for DogStatsD traffic.

It's important to note that the size of the third set in the list above, "all components available", has no performance effect. As long as the components would be included in the binary anyway, it does no harm to make them available in the app.
