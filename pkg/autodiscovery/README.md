# package `autodiscovery`

This package is a core piece of the agent. It is responsible for collecting checks configurations from different sources (see package [config providers](https://github.com/DataDog/datadog-agent/tree/master/pkg/autodiscovery/providers)) and then schedule or unschedule integrations configurations with the help of the schedulers.

It is also responsible for listening to container-related events and resolve template configurations that would match them.

## `AutoConfig`

As a central component, `AutoConfig` owns and orchestrates several key modules:

- it owns a reference to a [`MetaScheduler`](https://github.com/DataDog/datadog-agent/blob/master/pkg/autodiscovery/scheduler) that dispatches integrations configs for scheduling or unscheduling to all registered schedulers.
- it stores a list of [`ConfigProviders`](https://github.com/DataDog/datadog-agent/blob/master/pkg/autodiscovery/providers) and poll them according to their policy
- it owns [`ServiceListener`](https://github.com/DataDog/datadog-agent/blob/master/pkg/autodiscovery/listeners) that it uses to listen to container lifecycle events
- it runs the `ConfigResolver` that resolves a configuration template to an actual configuration based on data it extracts from a service that matches it the template

## ConfigResolver

`ConfigResolver` resolves configuration templates with services and asks `AutoConfig` to schedule checks based on this resolving.

To fulfill its task, it stores a cache of services (containers) and templates. It also keeps a live map of how they apply to each other and of the checks that it scheduled as a result.

It also listens on three channels, one for fresh configuration templates, and two for newly started/stopped services.
