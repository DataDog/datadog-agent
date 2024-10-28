# Agent Sidecar Auto-Injection

## Overview

Agent sidecar auto-injection is implemented as a webhook in the DCA admission controller. A mutation function is defined to process the pod mutation request that is forwarded to the webhook on pod creation.

The main goal of this webhook is to facilitate the user experience when running in environments where the agent should be deployed as a sidecar.

The agent sidecar is injected on pods that match the webhook selector(s).

## Providers

We support the use of providers. In this context, a provider is tied to a specific runtime environment (e.g. fargate).

Currently, only `fargate` provider is supported.

A provider serves to auto-configure the injected agent sidecar to target the specified provider environment by setting some extra environment variables for example.

## Profiles

A profile defines a set of overrides that the user would like to apply to the agent sidecar such as environment variables and/or resource limits.

## Configuration Modes

The configuration of the webhook depends on the user needs and can go from simple configuration to complex and advanced configuration.

### Simplest Configuration

The minimum requirement to activate this feature includes the following:
- Enabling the feature
- Setting the provider
- Creating datadog secrets in every namespace where you wish to inject the agent sidecar

With this configuration, all pods having the label `agent.datadoghq.com/sidecar: <provider>` will be injected with an agent sidecar.

The injected sidecar will automatically have all the configuration needed for the specified provider.

### Custom Selectors/Profiles Without Provider

A more complex setup can include the following:
- Enabling the feature
- Setting custom selectors
- Setting custom profiles
- Creating datadog secrets in every namespace where you wish to inject the agent sidecar

This allows the user to customize the matching criteria for the webhook. It allows specifying which pods will be injected with the agent sidecar.

With this configuration, the default agent sidecar will be injected, in addition to any overrides set by the user in the specified profiles.

### Custom Selectors/Profiles Without Provider

This configuration includes the following:
- Enabling the feature
- Setting custom selectors
- Setting custom profiles
- Setting a provider
- Creating datadog secrets in every namespace where you wish to inject the agent sidecar

This allows the user to customize the matching criteria for the webhook. It allows specifying which pods will be injected with the agent sidecar.

With this configuration, the default agent sidecar will be injected, in addition to any overrides set by the user.

Having set a provider, the agent sidecar will also get automatically the necessary configurations for the targeted provider.

## Expected Behaviour

The table below shows the expected behaviour when the feature is enabled and valid selecors and/or profiles (or none) are provided.

Please note that in case of any misconfiguration of selectors or profiles, the webhook will not be registered.
A misconfiguration includes the following:
- Providing malformed configuration for selectors or for profiles
- Providing multiple selectors
- Providing multiple profiles

Note that an empty provider is valid, as it represents the absence of provider.

| Custom Selectors   | Profiles | Provider Set       | Provider Supported | Expected Behaviour                                                                                                                                                                                               |
|--------------------|----------|--------------------|--------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| :x:                | any      | :heavy_check_mark: | :heavy_check_mark: | Agent sidecar should be injected on pods having the provider label key set (`agent.datadoghq.com/sidecar: <provider>`). Any overrides specified in the `Profile` will take precedence on any conflicting default |
| :x:                | any      | :heavy_check_mark: | :x:                | No agent sidecar should be injected, and an error message will be logged in the cluster agent:  "agent sidecar provider is not supported: foo-provider"                                                          |
| :x:                | any      | :x:                | :heavy_check_mark: | No agent sidecar should be injected, and an error message will be logged in the cluster agent:  "agent sidecar provider is not supported"                                                                        |
| :heavy_check_mark: | any      | :heavy_check_mark: | :heavy_check_mark: | The agent sidecar container should be injected only on pods matching the selector, and the `DD_EKS_FARGATE` label should be set to `true`                                                                        |
| :heavy_check_mark: | any      | :heavy_check_mark: | :x:                | No Agent sidecar is injected, and you must find an error message in the cluster agent logs "agent sidecar provider is not supported:: foo"                                                                       |
| :heavy_check_mark: | any      | :x:                | :heavy_check_mark: | The agent sidecar container should be injected only on pods matching the selector                                                                                                                                |




## Notes
- For now, we only support configuring 1 custom selector and 1 custom profile.
- For now, only `fargate` provider is supported
- For now, only 1 selector and 1 profile (config override) can be configured.
- Configurations set by user via `Profiles` have the highest priority; they override any default configuration in case of conflict.
- An empty provider is valid, as it represents the absence of provider.
