## package `autodiscovery`

This package is a core piece of the agent. It is responsible for collecting checks configurations from different sources (see package [config providers](https://github.com/DataDog/datadog-agent/tree/master/pkg/collector/providers)) and then create, update or destroy check instances with the help of the Collector.

It is also responsible for listening to container-related events and trigger check scheduling decisions based on them.


### `AutoConfig`

As a central component, `AutoConfig` owns and orchestrates several key modules:

- it owns a reference to the `Collector` that it uses to (un)schedule checks when template or container updates warrant them
- it stores a list of `ConfigProviders` and poll them according to their policy
- it owns and uses [check loaders](https://github.com/DataDog/datadog-agent/tree/haissam/docker-listener/pkg/collector/check#check-loaders) to load configurations into `Check` objects
- it owns [listeners](https://github.com/DataDog/datadog-agent/tree/haissam/docker-listener/pkg/collector/listeners) that it uses to listen to container lifecycle events
- it runs the `ConfigResolver` that resolves a configuration template to an actual configuration based on data it extracts from a service that matches it the template

**TODO:**
- `pollConfigs` needs to send collected templates to ConfigResolver.FreshTemplates.
- processTemplates needs to work on partial updates and not just full template lists.


### ConfigResolver

`ConfigResolver` resolves configuration templates with services and asks `AutoConfig` to schedule checks based on this resolving.

To fulfill its task, it stores a cache of services (containers) and templates. It also keeps a live map of how they apply to each other and of the checks that it scheduled as a result.

It also listens on three channels, one for fresh configuration templates, and two for newly started/stopped services.

**TODO**:
- ConfigResolver is responsible for too many things. Scheduling should go back to AutoConfig, or get its own module.
- getters for template variables are all placeholder, they need to be implemented. Tags should just return svc.Tags, host and port should consider key/idx
- IsConfigMatching is too simple. We need to re-implement the logic of agent5 matching
