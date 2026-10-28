# Remaining documentation

The [`doc/` directory](../doc/index.md) is the sole source of canonical developer documentation for those working on the Datadog Agent, published on the [developer docs site](https://datadoghq.dev/datadog-agent/). Add and maintain developer documentation in `doc/`.

This directory contains temporary pages that we are gradually reviewing for removal or incorporation into the canonical developer docs and files whose existing paths serve external consumers. None of these pages are published by the developer-site build. Keep externally consumed paths intact until their consumers have migrated.

## Temporary pages awaiting review

### Unpublished developer pages

These pages await individual review for removal or incorporation into the canonical docs:

- [Developer page index](dev/README.md)
- [Agent Data Plane](dev/agent-data-plane.md)
- [Agent API](dev/agent_api.md)
- [Development caveats](dev/caveats.md)
- [Go module replacements](dev/gomodreplace.md)
- [Kubernetes Autodiscovery annotations](dev/kubernetes-autodiscovery-annotations.md)
- [Legal](dev/legal.md)
- [Custom checks](dev/checks/README.md)
- [Checks: `_util`](dev/checks/builtins/_util.md)
- [Checks: `aggregator`](dev/checks/builtins/aggregator.md)
- [Checks: `containers`](dev/checks/builtins/containers.md)
- [Checks: `datadog_agent`](dev/checks/builtins/datadog_agent.md)
- [Checks: `kubeutil`](dev/checks/builtins/kubeutil.md)
- [Checks: `tagger`](dev/checks/builtins/tagger.md)
- [Checks: `util`](dev/checks/builtins/util.md)

### Deprecated component guidance

These pages contain deprecated component guidance:

- [Defining apps](public/guidelines/deprecated-components-documentation/defining-apps.md)
- [Defining bundles](public/guidelines/deprecated-components-documentation/defining-bundles.md)
- [Purpose](public/guidelines/deprecated-components-documentation/purpose.md)
- [Registrations](public/guidelines/deprecated-components-documentation/registrations.md)
- [Subscriptions](public/guidelines/deprecated-components-documentation/subscriptions.md)
- [Using components](public/guidelines/deprecated-components-documentation/using-components.md)

### Empty placeholders

These pages contain only a title and a TODO comment:

- [Component metadata](public/how-to/components/metadata.md)
- [Component remote configuration](public/how-to/components/remote-config.md)
- [Component workload metadata](public/how-to/components/workloadmeta.md)

### Security documentation generation

The [generation instructions](cloud-workload-security/README.md) remain beside their artifacts pending review for incorporation into the canonical docs.

## External documentation and artifacts

| Page or file | Why its path is preserved |
| --- | --- |
| [Configured hostnames](public/hostname/hostname_force_config_as_canonical.md) | The [short link emitted by Agent warnings](https://dtdg.co/agent-hostname-force-config-as-canonical) resolves to this exact GitHub path. |
| [Secret executable permissions](public/secrets/Set-SecretPermissions.ps1), [secret executable tester](public/secrets/secrets_tester.ps1) | The [corporate secrets documentation](https://docs.datadoghq.com/agent/configuration/secrets-management/) links directly to these scripts. |
| [Agent expressions](cloud-workload-security/agent_expressions.md), [backend events](cloud-workload-security/backend.md) | These older generated security references remain alongside the current outputs for compatibility. |
| [Linux backend events](cloud-workload-security/backend_linux.md), [Windows backend events](cloud-workload-security/backend_windows.md) | These generated security references remain with their schemas and generation workflows. |
| [Linux expressions](cloud-workload-security/linux_expressions.md), [Windows expressions](cloud-workload-security/windows_expressions.md), [Workload Protection Agent configuration](cloud-workload-security/workload_protection_agent_config.md) | These generated customer-facing security references remain at their existing consumer paths. |

The JSON schemas, SECL model JSON files, and `BUILD.bazel` in `cloud-workload-security/` stay with these outputs to preserve generation targets and consumer paths.

## User documentation

- [Datadog Agent](https://docs.datadoghq.com/agent/) documents installation and operation.
- [DogStatsD](https://docs.datadoghq.com/developers/dogstatsd/) documents metrics submission.
- [Datadog Cluster Agent](https://docs.datadoghq.com/containers/cluster_agent/) documents cluster-level operation.
- The [Agent](../Dockerfiles/agent/README.md), [DogStatsD](../Dockerfiles/dogstatsd/alpine/README.md), and [Cluster Agent](../Dockerfiles/cluster-agent/README.md) images have additional documentation beside their Dockerfiles.
