# package `autodiscovery`

This package is a core piece of the agent. It is responsible for collecting checks configurations from different sources (see package [config providers](https://github.com/DataDog/datadog-agent/tree/master/pkg/autodiscovery/providers)) and then schedule or unschedule integrations configurations with the help of the schedulers.

It is also responsible for listening to container-related events and resolve template configurations that would match them.

## `AutoConfig`

As a central component, `AutoConfig` owns and orchestrates several key modules:

- it owns a reference to a [`MetaScheduler`](https://github.com/DataDog/datadog-agent/blob/master/pkg/autodiscovery/scheduler) that dispatches integrations configs for scheduling or unscheduling to all registered schedulers.
- it stores a list of [`ConfigProviders`](https://github.com/DataDog/datadog-agent/blob/master/pkg/autodiscovery/providers) and poll them according to their policy
- it owns [`ServiceListener`](https://github.com/DataDog/datadog-agent/blob/master/pkg/autodiscovery/listeners) used to listen to container lifecycle events and listen to them
- it uses the `ConfigResolver` that resolves a configuration template to an actual configuration based on a service matching the template
- it uses a `store` component to safely store and retrieve all data and mappings needed for the autodiscovery lifecycle

## ConfigResolver

`ConfigResolver` resolves configuration templates with services.
