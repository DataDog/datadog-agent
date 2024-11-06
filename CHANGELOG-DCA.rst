=============
Release Notes
=============

.. _Release Notes_7.58.2:

7.58.2
======

.. _Release Notes_7.58.2_Prelude:

Prelude
-------

Released on: 2024-11-04
Pinned to datadog-agent v7.58.2: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7582>`_.

.. _Release Notes_7.58.1:

7.58.1
======

.. _Release Notes_7.58.1_Prelude:

Prelude
-------

Released on: 2024-10-24
Pinned to datadog-agent v7.58.1: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7581>`_.

.. _Release Notes_7.58.0:

7.58.0
======

.. _Release Notes_7.58.0_Prelude:

Prelude
-------

Released on: 2024-10-21
Pinned to datadog-agent v7.58.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7580>`_.

.. _Release Notes_7.58.0_New Features:

New Features
------------

- Added capability to tag any Kubernetes resource based on labels and annotations.
  This feature can be configured with `kubernetes_resources_annotations_as_tags` and `kubernetes_resources_labels_as_tags`.
  These feature configurations are associate group resources with annotations-to-tags (or labels-to-tags) map
  For example, `deployments.apps` can be associated with an annotations-to-tags map to configure annotations as tags for deployments.
  Example:
  {`deployments.apps`: {`annotationKey1`: `tag1`, `annotationKey2`: `tag2`}}

- The Kubernetes State Metrics (KSM) check can now be configured to collect
  pods from the Kubelet in node agents instead of collecting them from the API
  Server in the Cluster Agent or the Cluster check runners. This is useful in
  clusters with a large number of pods where emitting pod metrics from a
  single check instance can cause performance issues due to the large number
  of metrics emitted.


.. _Release Notes_7.58.0_Enhancement Notes:

Enhancement Notes
-----------------

- Added a new option for the Cluster Agent
  ("admission_controller.inject_config.type_socket_volumes") to specify that
  injected volumes should be of type "Socket". This option is disabled by
  default. When set to true, injected pods will not start until the Agent
  creates the DogstatsD and trace-agent sockets. This ensures no traces or
  DogstatsD metrics are lost, but it can cause the pod to wait if the Agent
  has issues creating the sockets.


.. _Release Notes_7.58.0_Bug Fixes:

Bug Fixes
---------

- Fixed an issue that prevented the Kubernetes autoscaler from evicting pods
  injected by the Admission Controller.
  

.. _Release Notes_7.57.1:

7.57.1
======

.. _Release Notes_7.57.1_Prelude:

Prelude
-------

Released on: 2024-09-17
Pinned to datadog-agent v7.57.1: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7571>`_.

.. _Release Notes_7.57.0:


7.57.0
======

.. _Release Notes_7.57.0_Prelude:

Prelude
-------

Released on: 2024-09-09
Pinned to datadog-agent v7.57.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7570>`_.

.. _Release Notes_7.57.0_New Features:

New Features
------------

- The Cluster Agent now supports activating Continuous Profiling
  using Admission Controller.

- ``LimitRange`` and ``StorageClass`` resources are now collected by the orchestrator check.


.. _Release Notes_7.57.0_Enhancement Notes:

Enhancement Notes
-----------------

- The auto-instrumentation webhook (beta) uses a new injector library.


.. _Release Notes_7.57.0_Bug Fixes:

Bug Fixes
---------

- Fixes a rare bug where some Kubernetes events would be emitted
  without a timestamp and would be dropped upstream as a result.

- Library package versions for auto-instrumentation are now set to the latest major
  version of the library-package instead of `latest`.
  
  * java:v1
  * dotnet:v2
  * python:v2
  * ruby:v2
  * js:v5

- Fix APIServer error logs generated when external metrics endpoint is activated


.. _Release Notes_7.56.2:

7.56.2
======

.. _Release Notes_7.56.2_Prelude:

Prelude
-------

Released on: 2024-09-02
Pinned to datadog-agent v7.56.2: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7562>`_.

.. _Release Notes_7.56.1:

7.56.1
======

.. _Release Notes_7.56.1_Prelude:

Prelude
-------

Released on: 2024-08-29
Pinned to datadog-agent v7.56.1: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7561>`_.

.. _Release Notes_7.56.0:

7.56.0
======

.. _Release Notes_7.56.0_Prelude:

Prelude
-------

Released on: 2024-08-16
Pinned to datadog-agent v7.56.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7560>`_.

.. _Release Notes_7.56.0_Upgrade Notes:

Upgrade Notes
-------------

- Disables default injection of the .NET profiler dependency for Kubernetes auto_instrumentation.


.. _Release Notes_7.56.0_Enhancement Notes:

Enhancement Notes
-----------------

- Mark the NetworkPolicy collector as stable in the Cluster Agent

- Enabled language detection automatically in the injected agent sidecar on EKS Fargate when APM SSI is enabled.
  This is only available for users using the admission controller to automatically inject the agent sidecar.

- The orchestrator check can now scrub sensitive data from probes in pods specifications.


.. _Release Notes_7.56.0_Bug Fixes:

Bug Fixes
---------

- Fixes issue where the external metrics server would sometimes return metrics which had not
  been updated for longer than the configured `external_metrics_provider.max_age` as valid.
  In connection with this fix, a new config (`external_metrics_provider.query_validity_period`)
  has been added to account for the delay between when metrics are resolved and when they
  are queried by the various autoscaling controllers. It is set to 30 seconds by default.

.. _Release Notes_7.55.3:

7.55.3
================

.. _Release Notes_7.55.3_Prelude:

Prelude
-------

Released on: 2024-08-01
Pinned to datadog-agent v7.55.3: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7553>`_.

.. _Release Notes_7.55.2:

7.55.2
================

.. _Release Notes_7.55.2_Prelude:

Prelude
-------

Released on: 2024-07-25
Pinned to datadog-agent v7.55.2: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7552>`_.

.. _Release Notes_7.55.1:

7.55.1
================

.. _Release Notes_7.55.1_Prelude:

Prelude
-------

Released on: 2024-07-12
Pinned to datadog-agent v7.55.1: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7551>`_.

.. _Release Notes_7.55.0:

7.55.0
======

.. _Release Notes_7.55.0_Prelude:

Prelude
-------

Released on: 2024-07-11
Pinned to datadog-agent v7.55.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7550>`_.

.. _Release Notes_7.55.0_New Features:

New Features
------------

- Add support for `kubernetes_namespace_annotations_as_tags`.
  This new option is equivalent to the existing `kubernetes_namespace_labels_as_tags`,
  but it considers namespace annotations instead of namespace labels.
  With this new option, users can enrich tagging based on namespace annotations.

- Support namespace labels as tags on kubernetes events.


.. _Release Notes_7.55.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add ``reason:backofflimitexceeded,deadlineexceeded`` to the
  ``kubernetes_state.job.failed`` metric to help users understand why a job failed.

- Reduced the memory used to store the tags.

- The Datadog cluster-agent container image is now using Ubuntu 24.04 noble
  as the base image.


.. _Release Notes_7.55.0_Bug Fixes:

Bug Fixes
---------

- Fixes an issue with large clusters where the Cluster
  Agent fails to collect all tags when
  `cluster_agent.collect_kubernetes_tags` is enabled.


.. _Release Notes_7.54.1:

7.54.1
================

.. _Release Notes_7.54.1_Prelude:

Prelude
-------

Released on: 2024-06-18
Pinned to datadog-agent v7.54.1: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7541>`_.

.. _Release Notes_7.54.0:

7.54.0
================

.. _Release Notes_7.54.0_Prelude:

Prelude
-------

Released on: 2024-05-29
Pinned to datadog-agent v7.54.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7540>`_.


.. _Release Notes_7.54.0_New Features:

New Features
------------

- Add `LimitRange` and `StorageClass` collection in the orchestrator check.


.. _Release Notes_7.54.0_Enhancement Notes:

Enhancement Notes
-----------------

- Added retry mechanism to language detection patcher in order to retry failed patching operations.


.. _Release Notes_7.54.0_Bug Fixes:

Bug Fixes
---------

- Fix collection of numeric rolling update options in Kubernetes deployments
  and daemonsets.

- Fixed initialization of language expiration time for detected languages.


.. _Release Notes_7.53.0:

7.53.0 / 6.53.0
================

.. _Release Notes_7.53.0_Prelude:

Prelude
-------

Released on: 2024-04-30
Pinned to datadog-agent v7.53.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7530>`_.


.. _Release Notes_7.53.0_New Features:

New Features
------------

- APM library injection now works on EKS Fargate when the admission controller
  is configured to add an Agent sidecar in EKS Fargate.

- Cluster Agent now supports activating Application Security Management, Code Vulnerabilities, and
  Software Composition Analysis via Helm charts.


.. _Release Notes_7.53.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add the `mutation_webhook` tag to `admission_webhooks.webhooks_received` and `admission_webhooks.response_duration` Cluster Agent telemetry.

- When using the admission controller to inject an Agent sidecar on EKS
  Fargate, `shareProcessNamespace` is now set to `true` automatically. This is
  to ensure that the process collection feature works.


.. _Release Notes_7.52.0:

7.52.0 / 6.52.0
================

.. _Release Notes_7.52.0_New Features:

New Features
------------

- Add agent sidecar injection webhook in `cluster-agent` Kubernetes admission controller. This new webhook adds the Agent as sidecar container in applicative Pods when it is required by the environment. For example with the EKS Fargate environment.


.. _Release Notes_7.52.0_Enhancement Notes:

Enhancement Notes
-----------------

- Introduces a new config option in the Cluster Agent to set the rebalance
  period when advanced dispatching is enabled:
  ``cluster_checks.rebalance_period``. The default value is 10 min.


.. _Release Notes_7.52.0_Bug Fixes:

Bug Fixes
---------

- Fix an issue where the admission controller would remove the field `restartPolicy`
  from native sidecar containers, preventing pod creation on Kubernetes 1.29+.

- Fix missing `kube_api_version` tag on HPA and VPA resources.


.. _Release Notes_7.51.0:

7.51.0 / 6.51.0
================

.. _Release Notes_7.51.0_Prelude:

Prelude
-------

Released on: 2024-02-19
Pinned to datadog-agent v7.51.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7510>`_.

.. _Release Notes_7.51.0_New Features:

New Features
------------

- Enable Horizontal Pod Autoscaler collection for the Orchestrator by default

- Add isolate command to clusterchecks to make it easier to pinpoint
  a check that that is causing high CPU/memory usage. Command can be
  run in the cluster agent with:
  `datadog-cluster-agent clusterchecks isolate --checkID=<checkID>`


.. _Release Notes_7.51.0_Enhancement Notes:

Enhancement Notes
-----------------

- Enable CRD collection by default in the orchestrator check.


.. _Release Notes_7.51.0_Bug Fixes:

Bug Fixes
---------

- Fixes a bug that would trigger unnecessary APIServer `List` requests from the Cluster Agent or Cluster Checks Runner.


.. _Release Notes_7.50.1:

7.50.1 / 6.50.1
================

.. _Release Notes_7.50.1_Bug Fixes:

Bug Fixes
---------

- Fixes a bug introduced in `7.50.0` preventing `DD_TAGS` to be added to `kubernetes_state.*` metrics.


.. _Release Notes_7.50.0:

7.50.0 / 6.50.0
================

.. _Release Notes_7.50.0_New Features:

New Features
------------

- Add language detection API handler to the cluster-agent.

- Report `rate_limit_queries_remaining_min` telemetry from `external-metrics` server.

- Added a new `--force` option to the `datadog-cluster-agent clusterchecks rebalance` command that allows you to force clustercheck rebalancing with utilization.

- [Beta] Enable `APM` library injection in `cluster-agent` admission controller based on automatic language detection annotations.


.. _Release Notes_7.50.0_Enhancement Notes:

Enhancement Notes
-----------------

- Show Autodiscovery information in the output of ``datadog-cluster-agent status``.

- Added CreateContainerConfigError wait reason to the `kubernetes_state.container.status_report.count.waiting` metric
  reported by the kubernetes_state_core check.

- Release the Leader Election Lock on shutdown to make the initialization of future cluster-agents faster.

- The Datadog cluster-agent container image is now using Ubuntu 23.10 mantic
  as the base image.


.. _Release Notes_7.50.0_Bug Fixes:

Bug Fixes
---------

- Fixed a bug in the ``kubernetes_state_core`` check that caused tag corruption when ``telemetry`` was set to ``true``.

- Fix stale metrics being reported by kubernetes_state_core check in some rare cases.

- Fixed a bug in the rebalancing of cluster checks. Checks that contained
  secrets were never rebalanced when the Cluster Agent was configured to not
  resolve check secrets (option ``secret_backend_skip_checks`` set to true).


.. _Release Notes_7.49.0:

7.49.0 / 6.49.0
================

.. _Release Notes_7.49.0_New Features:

New Features
------------

- Added option to attach profiling data to a flare.

- Increment cluster agent admission controller mutation attempts metric when library is auto-injected.


.. _Release Notes_7.49.0_Enhancement Notes:

Enhancement Notes
-----------------

- Added the ``check_name`` tag to the ``cluster_checks.configs_info`` metric emitted by the Cluster Agent telemetry.

- Sensitive information is now scrubbed from pod annotations.

- Skip collections for resources missing RBACs in orchestrator check


.. _Release Notes_7.49.0_Bug Fixes:

Bug Fixes
---------

- Remove openmetrics endpoint default value from `containerd` check default configuration.

- Resolved a conflict between the admission controller and the AKS admissions enforcer that previously led to a loop in reconciling the webhook.

- Fixes a panic in the Cluster Agent that happens when trying to unschedule a
  check that has not been dispatched to any runner.


.. _Release Notes_7.48.0:

7.48.0 / 6.48.0
================

.. _Release Notes_7.48.0_New Features:

New Features
------------

- Added the kubernetes_state.pod.tolerations metric to the KSM core check

- Add ``HorizontalPodAutoscaler`` collection in the orchestrator check.


.. _Release Notes_7.48.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add safeguards for orchestrator CRD collection.

- The Datadog cluster-agent container image is now using Ubuntu 23.04 lunar
  as the base image.


.. _Release Notes_7.48.0_Bug Fixes:

Bug Fixes
---------

- Fixed an error in the calculations performed by the algorithm that rebalances cluster checks. Cluster checks are now more evenly distributed when advanced dispatching is enabled (``cluster_checks.advanced_dispatching_enabled`` is set to true).

- Service checks are no longer excluded from rebalancing decisions when advanced dispatching is enabled (``cluster_checks.advanced_dispatching_enabled`` is set to true).

- Fixes a rare bug in the Kubernetes State check that causes the Agent to incorrectly tag the ``kubernetes_state.job.complete`` service check.

- Removes an incorrect warning log message that mentions that the DD_POD_NAME
  env var is unknown.

- Fixes the KSM check to support HPA v2beta2 again. This stopped working in Agent v7.44.0.

- Adds the `kube_cluster_name` tag as a static global tag to the cluster agent when the `DD_CLUSTER_NAME` config option is set.
  This should fix an issue where the tag is not being attached to metrics in certain environments, such as EKS Fargate.

- Fixed a bug in the advanced dispatching of cluster checks. All the checks
  scheduled since the last rebalance were being scheduled in the same node.
  Now they should be distributed among the available nodes.


.. _Release Notes_7.47.0:

7.47.0 / 6.47.0
================

.. _Release Notes_7.47.0_Upgrade Notes:

Upgrade Notes
-------------

- Add support for leases in leader election which can be enabled by setting
  ``leader_election_default_resource`` to ``leases``, available since Kubernetes version 1.14.
  If this parameter is empty, leader election automatically detects if leases
  are available and uses them.
  Set ``leader_election_default_resource`` to ``configmap`` on clusters running
  Kubernetes versions previous to 1.14.


.. _Release Notes_7.47.0_New Features:

New Features
------------

- Auto-instrumentation admission controller now automatically activates crash tracking for Java applications


.. _Release Notes_7.47.0_Enhancement Notes:

Enhancement Notes
-----------------

- Expose to cluster-agent HistogramBuckets and Events check stats.
  It should help the cluster-agent to define a better cluster-checks
  dispatching.


.. _Release Notes_7.47.0_Bug Fixes:

Bug Fixes
---------

- The Cluster Agent Admission Controller now injects DD_DOGSTATSD_URL when used in `socket` mode (default), allowing DogStatsD clients to work without configuration.

- Fix persistent volume type for local volumes.


.. _Release Notes_7.46.0:

7.46.0 / 6.46.0
================

.. _Release Notes_7.46.0_New Features:

New Features
------------

- Enable collection of Vertical Pod Autoscalers by default in the orchestrator check.


.. _Release Notes_7.46.0_Enhancement Notes:

Enhancement Notes
-----------------

- Collect conditions for a variety of Kubernetes resources.

- Collect persistent volume source in the orchestrator check.


.. _Release Notes_7.46.0_Bug Fixes:

Bug Fixes
---------

- Fix the timeout for idle HTTP connections.

- When the cluster-agent is started with ``hostNetwork: true``, the leader election mechanism was using a node name instead of the pod name. This was breaking the “follower to leader” forwarding mechanism.
  This change introduce the ``DD_POD_NAME`` environment variable as a more reliable way to set the cluster-agent pod name. It is supposed to be filled by the Kubernetes downward API.


.. _Release Notes_7.45.0:

7.45.0 / 6.45.0
================

.. _Release Notes_7.45.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add "active" tag on the telemetry datadog.cluster_agent.external_metrics.datadog_metrics tag.
  The label active is true if DatadogMetrics CR is used, false otherwise.

- Library injection via Admission Controller: Allow configuring the CPU and Memory requests/limits for library init containers.

- Validate the orchestration config provided by the user.


.. _Release Notes_7.45.0_Bug Fixes:

Bug Fixes
---------

- Fix the admission controller in socket mode for pods with init containers.

- Fix resource requirements detection for containers without any request and
  limit set.

- The KSM core check now correctly handles labels and annotations with
  uppercase letters defined in the "labels_as_tags" and "annotations_as_tags"
  config attributes.


.. _Release Notes_7.44.0:

7.44.0 / 6.44.0
================

.. _Release Notes_7.44.0_New Features:

New Features
------------

- Add conditions to Vertical Pod Autoscalers

- Experimental: Support Ruby library injection through the Admission Controller on Kubernetes.


.. _Release Notes_7.44.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add new metrics for the KSM Core check for extended resources:
  - Pod requests and limits of the network bandwidth extended resource: `kubernetes_state.container.network_bandwidth_limit`, `kubernetes_state.container.network_bandwidth_requested`
  - The capacity and allocatable network bandwidth extended resource of a node: `kubernetes_state.node.network_bandwidth_allocatable`, `kubernetes_state.node.network_bandwidth_capacity`

- Admission Controller: Add telemetry around auto-instrumentation via remote config.

- The UDS socket volume when using the Admission Controller is now mounted in readOnly mode.


.. _Release Notes_7.43.0:

7.43.0 / 6.43.0
================

.. _Release Notes_7.43.0_New Features:

New Features
------------

- Starts the collecting of Vertical Pod Autoscalers within Kubernetes clusters.

- Enable orchestrator manifest collection by default


.. _Release Notes_7.43.0_Bug Fixes:

Bug Fixes
---------

- Make the cluster-agent admission controller able to inject libraries for several languages in a single pod.


.. _Release Notes_7.42.0:

7.42.0 / 6.42.0
================

.. _Release Notes_7.42.0_New Features:

New Features
------------

- Supports the collection of custom resource definition and custom resource manifests for the orchestrator explorer.


.. _Release Notes_7.42.0_Enhancement Notes:

Enhancement Notes
-----------------

- Collects Unified Service Tags for the orchestrator explorer product.


.. _Release Notes_7.41.0:

7.41.0 / 6.41.0
================

.. _Release Notes_7.41.0_New Features:

New Features
------------

- Add ``Namespace`` collection in the orchestrator check and enable it by default.


.. _Release Notes_7.41.0_Enhancement Notes:

Enhancement Notes
-----------------

- Improves performance of the Cluster Agent admission controller on large pods.


.. _Release Notes_7.40.0:

7.40.0 / 6.40.0
================

.. _Release Notes_7.40.0_New Features:

New Features
------------

- Experimental: The Datadog Admission Controller can inject the Python APM library into Kubernetes containers for auto-instrumentation.

- The orchestrator check is now able to discover resources to collect based
  on API groups available in the Kubernetes cluster.


.. _Release Notes_7.40.0_Enhancement Notes:

Enhancement Notes
-----------------

- The admission controller now injects variables and volume mounts to init containers in addition to regular containers.

- Chunk orchestrator payloads by size and weight

- KSM Core check: Add the ``helm_chart`` tag automatically from the standard helm label ``helm.sh/chart``.

- Helm check: Add a ``helm_chart`` tag, equivalent to the standard helm label ``helm.sh/chart`` (see https://helm.sh/docs/chart_best_practices/labels/).


.. _Release Notes_7.40.0_Bug Fixes:

Bug Fixes
---------

- Fixed an edge case in the Admission Controller when ``mutateUnlabelled`` is enabled and ``configMode`` is set to ``socket``.
  This combination could prevent the creation of new DaemonSet Agent pods.

- Fixed a resource leak in the helm check.


.. _Release Notes_7.39.0:

7.39.0 / 6.39.0
================

.. _Release Notes_7.39.0_New Features:

New Features
------------

- Experimental: The Datadog Admission Controller can inject the Node and Java APM libraries into Kubernetes containers for auto-instrumentation.


.. _Release Notes_7.39.0_Enhancement Notes:

Enhancement Notes
-----------------

- When injecting env vars with the admission controller, env
  vars are now prepended instead of appended, meaning that
  Kubernetes [dependent environment variables](https://kubernetes.io/docs/tasks/inject-data-application/define-interdependent-environment-variables/)
  can now depend on these injected vars.

- The ``helm`` check has new configuration parameters:
  - ``extra_sync_timeout_seconds`` (default 120)
  - ``informers_resync_interval_minutes`` (default 10)

- Improves the `labelsAsTags` feature of the Kubernetes State Metrics core check by performing the transformations of characters ['/' , '-' , '.']
  to underscores ['_'] within the Datadog agent.
  Previously users had to perform these conversions manually in order to discover the labels on their resources.


.. _Release Notes_7.39.0_Bug Fixes:

Bug Fixes
---------

- Fix the DCA ``leader_election_is_leader`` metric that could sometimes report ``is_leader="false"`` on the leader instance

- Fixed an error when running ``datadog-cluster-agent status`` with
  ``DD_EXTERNAL_METRICS_PROVIDER_ENABLED=true`` and no app key set.

- The KSM Core check now handles cron job schedules with time zones.


.. _Release Notes_7.39.0_Other Notes:

Other Notes
-----------

- Align Cluster Agent version to Agent version. Cluster Agent will now be released with 7.x.y tags


.. _Release Notes_dca-1.22.0_dca-1.22.X:

dca-1.22.0
==========

.. _Release Notes_dca-1.22.0_dca-1.22.X_Prelude:

Prelude
-------

Released on: 2022-07-26
Pinned to datadog-agent v7.38.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7380--6380>`_.

.. _Release Notes_dca-1.22.0_dca-1.22.X_New Features:

New Features
------------

- Enable collection of Ingresses by default in the orchestrator check.

.. _Release Notes_dca-1.21.0_dca-1.21.X:

dca-1.21.0
==========

.. _Release Notes_dca-1.21.0_dca-1.21.X_Prelude:

Prelude
-------

Released on: 2022-06-28
Pinned to datadog-agent v7.37.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7370--6370>`_.

.. _Release Notes_dca-1.21.0_dca-1.21.X_Enhancement Notes:

Enhancement Notes
-----------------

- The Cluster Agent followers now forward queries to the Cluster Agent leaders themselves. This allows a reduction in the overall number of connections to the Cluster Agent and better spreads the load between leader and forwarders.

- Make the name of the ConfigMap used by the Cluster Agent for its leader election configurable.

- The Datadog Cluster Agent exposes a new metric ``endpoint_checks_configs_dispatched``.


.. _Release Notes_dca-1.21.0_dca-1.21.X_Bug Fixes:

Bug Fixes
---------

- Fix a panic occuring during the invocation of the `check` command on the
  Cluster Agent if the Orchestrator Explorer feature is enabled.

- Fix the node count reported for Kubernetes clusters.


.. _Release Notes_dca-1.20.0_dca-1.20.X:

dca-1.20.0
==========

.. _Release Notes_dca-1.20.0_dca-1.20.X_Prelude:

Prelude
-------

Released on: 2022-05-22
Pinned to datadog-agent v7.36.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7360--6360>`_.

.. _Release Notes_dca-1.20.0_dca-1.20.X_New Features:

New Features
------------

- The Datadog Admission Controller supports multiple configuration injection
  modes through the ``admission_controller.inject_config.mode`` parameter
  or the ``DD_ADMISSION_CONTROLLER_INJECT_CONFIG_MODE`` environment variable:
  - ``hostip``: Inject the host IP. (default)
  - ``service``: Inject Datadog's local-service DNS name.
  - ``socket``: Inject the Datadog socket path.

- Collect ResourceRequirements for jobs and cronjobs for kubernetes live containers.


.. _Release Notes_dca-1.20.0_dca-1.20.X_Enhancement Notes:

Enhancement Notes
-----------------

- Added a configuration option to admission controller to allow
  configuration of the failure policy. Defaults to Ignore which
  was the previous default. The default of Ignore means that pods
  will still be admitted even if the webhook is unavailable to
  inject them. Setting to Fail will require the admission controller
  to be present and pods to be injected before they are allowed to run.

- The admission controller's reinvocation policy is now set to ``IfNeeded`` by default.
  It can be changed using the ``admission_controller.reinvocation_policy`` parameter.

- The Datadog Cluster Agent now supports internal profiling.

- KSM core check: add a new ``kubernetes_state.cronjob.complete``
  service check that returns the status of the most recent job for
  a cronjob.


.. _Release Notes_dca-1.20.0_dca-1.20.X_Security Notes:

Security Notes
--------------

- Cluster Agent API (only used by Node Agents) is now only server with TLS >= 1.3 by default. Setting "cluster_agent.allow_legacy_tls" to true allows to fallback to TLS 1.0.


.. _Release Notes_dca-1.20.0_dca-1.20.X_Bug Fixes:

Bug Fixes
---------

- Fix the node count reported for Kubernetes clusters.

- Fixed an issue that created lots of log messages when the DCA admission controller was enabled on AKS.

- Time-based metrics (for example, `kubernetes_state.pod.age`, `kubernetes_state.pod.uptime`) are now comparable in the Kubernetes state core check.

- Fix a risk of panic when multiple KSM Core check instances run concurrently.

- Remove noisy Kubernetes API deprecation warnings in the Cluster Agent logs.


.. _Release Notes_dca-1.20.0_dca-1.20.X_Other Notes:

Other Notes
-----------

- Change the default value of the external metrics provider port from 443 to 8443.
  This will allow to run the cluster agent with a non-root user for better security.
  This was already the default value in the Helm chart and in the datadog operator.


.. _Release Notes_dca-1.19.0_dca-1.19.X:

dca-1.19.0
==========

.. _Release Notes_dca-1.19.0_dca-1.19.X_Prelude:

Prelude
-------

Released on: 2022-04-12
Pinned to datadog-agent v7.35.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7350--6350>`_.

.. _Release Notes_dca-1.19.0_dca-1.19.X_New Features:

New Features
------------

- Collect ResourceRequirements on other K8s workloads as well for live containers (Deployment, StatefulSet, ReplicaSet, DaemonSet)
- Enable collection of Roles/RoleBindings/ClusterRoles/ClusterRoleBindings/ServiceAccounts by default in the orchestrator check.
- Add ``Ingress`` collection in the orchestrator check.

.. _Release Notes_dca-1.19.0_dca-1.19.X_Bug Fixes:

Bug Fixes
---------

- Fix a bug that prevents scrubbing sensitive content on the DaemonSet resource.
- Fix a bug that prevents scrubbing sensitive content on the StatefulSet resource.

.. _Release Notes_dca-1.19.0_dca-1.19.X_Enhancement Notes:

Enhancement Notes
-----------------

- Adds a new histogram metric `admission_webhooks_response_duration` to monitor the admission-webhook's response time. The existing metric `admission_webhooks_webhooks_received` is now a counter.
- The cluster agent has an external metrics provider feature to allow using Datadog queries in Kubernetes HorizontalPodAutoscalers.
    It sometimes faces issues like:
    2022-01-01 01:01:01 UTC | CLUSTER | ERROR | (pkg/util/kubernetes/autoscalers/datadogexternal.go:79 in queryDatadogExternal) | Error while executing metric query ... truncated... API returned error: Query timed out
    To mitigate this problem, use the new ``external_metrics_provider.chunk_size`` parameter to reduce the number of queries that are batched by the Agent and sent together to Datadog.

.. _Release Notes_dca-1.18.0_dca-1.18.X:

dca-1.18.0
==========

.. _Release Notes_dca-1.18.0_dca-1.18.X_Prelude:

Prelude
-------

Released on: 2022-03-01
Pinned to datadog-agent v7.34.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7340--6340>`_.

.. _Release Notes_dca-1.18.0_dca-1.18.X_New Features:

New Features
------------

- Add an ``external_metrics_provider.endpoints`` parameter that allows to specify a list of external metrics provider endpoints.
If the first one fails, the DCA will query the next ones.
- Support file-based endpoint checks.
- Enable collection of PV/PVCs by default in the orchestrator check
- File-based cluster checks support Autodiscovery.

.. _Release Notes_dca-1.18.0_dca-1.18.X_Bug Fixes:

Bug Fixes
---------

- Fix the ``Admission Controller``/``Webhooks info`` section of the cluster agent ``agent status`` output on Kubernetes 1.22+.
Although the cluster agent was able to register its webhook with both the ``v1beta1`` and the ``v1`` version of the Administrationregistration API, the ``agent status`` command was always using the ``v1beta1``, which has been removed in Kubernetes 1.22.
- Improve error handling of deleted HPA objects.
- Fix an issue where scrubbing custom sensitive words would not work as intended for the orchestrator check.
- Fixed a bug that could prevent the Admission Controller from starting when the External Metrics Provider is enabled.
- Fix the caculation of orchestrator cache hits.


.. _Release Notes_dca-1.17.0_dca-1.17.X:

dca-1.17.0
==========

.. _Release Notes_dca-1.17.0_dca-1.17.X_Prelude:

Prelude
-------

Released on: 2022-01-26
Pinned to datadog-agent v7.33.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7330>`_.

.. _Release Notes_dca-1.17.0_dca-1.17.X_New Features:

New Features
------------

- Collect PVC tag on pending pods
- Add the ability to filter for check names in the cluster checks output.


.. _Release Notes_dca-1.17.0_dca-1.17.X_Bug Fixes:

Bug Fixes
---------

- Add reworked status output for orchestrator section on CLC setups.

.. _Release Notes_dca-1.17.0_dca-1.17.X_Security:

Security
--------

- Fix the removal of the "kubectl.kubernetes.io/last-applied-configuration" annotation on new collected resources

.. _Release Notes_dca-1.17.0_dca-1.17.X_Enhancement Notes:

Enhancement Notes
-----------------

- Add autoscaler resource kind (hpa,wpa) inside the DatadogMetrics status references.

.. _Release Notes_dca-1.16.0_dca-1.16.X:

dca-1.16.0
==========

.. _Release Notes_dca-1.16.0_dca-1.16.X_Prelude:

Prelude
-------

Released on: 2021-11-10
Pinned to datadog-agent v7.32.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7320>`_.

.. _Release Notes_dca-1.16.0_dca-1.16.X_New Features:

New Features
------------

- Introduce the collection of the following resources: ClusterRole, ClusterRoleBinding, Role, RoleBinding, ServiceAccount.

.. _Release Notes_dca-1.16.0_dca-1.16.X_Bug Fixes:

Bug Fixes
---------

- Fix tags for PV resources in the Orchestrator Explorer (type and phase).
- Fix an edge case in which the Cluster Agent's Admission Controller doesn't update the Webhook object according to specified configuration.

.. _Release Notes_dca-1.15.0_dca-1.15.X:

dca-1.15.0
==========

.. _Release Notes_dca-1.15.0_dca-1.15.X_Prelude:

Prelude
-------

Released on: 2021-09-13
Pinned to datadog-agent v7.31.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7310>`_.

.. _Release Notes_dca-1.15.0_dca-1.15.X_New Features:

New Features
------------

- Enable ``StatefulSet`` collection by default in the orchestrator check.
- Add ``PV`` and ``PVC`` collection in the orchestrator check.
- Added possibility to use the `maxAge` attribute defined in the datadogMetric CRD overriding the global `maxAge`.


.. _Release Notes_dca-1.14.0_dca-1.14.X:

dca-1.14.0
==========

.. _Release Notes_dca-1.14.0_dca-1.14.X_Prelude:

Prelude
-------

Released on: 2021-08-12
Pinned to datadog-agent v7.30.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7300>`_.

.. _Release Notes_dca-1.14.0_dca-1.14.X_New Features:

New Features
------------

- Enable ``DaemonSet`` collection by default in the orchestrator check. Add ``StatefulSet`` collection in the orchestrator check.

.. _Release Notes_dca-1.14.0_dca-1.14.X_Enhancement Notes:

Enhancement Notes
-----------------

- The Cluster Agent's Admission Controller now uses the ``admissionregistration.k8s.io/v1`` kubernetes API when available.
- The Cluster Agent can be instructed to dispatch cluster checks without decrypting secrets. The node Agent or the cluster check runner will fetch the secrets after receiving the configurations from the Cluster Agent. This can be enabled by setting ``DD_SECRET_BACKEND_SKIP_CHECKS`` to ``true`` in the Cluster Agent config.
- The Cluster Agent's external metrics provider now serves an OpenAPI endpoint.
- Add the ability to change log_level at runtime. To set the log_level to ``debug`` the following command should be used: ``agent config set log_level debug``.
- Improve status and flare for the Cluster Check Runners.

.. _Release Notes_dca-1.14.0_dca-1.14.X_Bug Fixes:

Bug Fixes
---------

- Show different orchestrator status collection information between follower and leader.
- Fix an edge case where the Admission Controller doesn't update the certificate according to the Cluster Agent configuration.

.. _Release Notes_dca-1.13.1_dca-1.13.X:

dca-1.13.1
==========

.. _Release Notes_dca-1.13.1_dca-1.13.X_Prelude:

Prelude
-------

Released on: 2021-07-05
Pinned to datadog-agent v7.29.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7290>`_.

Bug Fixes
---------

- Fix the embedded security policy version to match the one from the agent.


.. _Release Notes_dca-1.13.0_dca-1.13.X:

dca-1.13.0
==========

.. _Release Notes_dca-1.13.0_dca-1.13.X_Prelude:

Prelude
-------

Released on: 2021-06-22
Pinned to datadog-agent v7.29.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7290>`_.


.. _Release Notes_dca-1.13.0_dca-1.13.X_New Features:

New Features
------------

- Collect the DaemonSet resources for the orchestrator explorer.


.. _Release Notes_dca-1.13.0_dca-1.13.X_Enhancement Notes:

Enhancement Notes
-----------------

- The Cluster Agent exposes a new metric `external_metrics.datadog_metrics` to track the validity of DatadogMetric objects.

- Add additional status information in orchestrator section output. Whether collection works and whether cluster name is set.


.. _Release Notes_dca-1.13.0_dca-1.13.X_Bug Fixes:

Bug Fixes
---------

- Autodetect EC2 cluster name

- Decrease the Admission Controller timeout to avoid edge cases where high timeouts can cause ignoring the ``failurePolicy`` (see kubernetes/kubernetes#71508).

- The Cluster Agent's admission controller now requires the pod label ``admission.datadoghq.com/enabled=true`` to inject standard labels. This optimizes the number of mutation webhook requests.


.. _Release Notes_dca-1.12.0_dca-1.12.X:

dca-1.12.0
==========

.. _Release Notes_dca-1.12.0_dca-1.12.X_Prelude:

Prelude
-------

  Pinned to datadog-agent v7.28.0-rc.5

.. _Release Notes_dca-1.12.0_dca-1.12.X_New Features:

New Features
------------

- The cluster-agent container now tries to remove any folder beginning by ``..`` in paths of
  files mounted in ``/conf.d`` while copying them to the cluster-agent config folder

- collect cluster resource for orchestrator explorer.

- It's now possible to template the kube_cluster_name tag in DatadogMetric queries
  Example: avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%tag_kube_cluster_name%%}

- It's now possible to template any environment variable (as seen by the Datadog Cluster Agent) as tag in DatadogMetric queries
  Example: avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%env_DD_CLUSTER_NAME%%}


.. _Release Notes_dca-1.12.0_dca-1.12.X_Enhancement Notes:

Enhancement Notes
-----------------

- It is now possible to configure a custom timeout for the MutatingWebhookConfigurations
  objects controlled by the Cluster Agent via DD_ADMISSION_CONTROLLER_TIMEOUT_SECONDS. (Default: 30 seconds)

- The Datadog Cluster Agent's Admission Controller now uses a namespaced secrets informer.
  It no longer needs permissions to watch secrets at the cluster scope.

- The cluster agent now uses the same configuration than the security agent for
  the logs endpoints configuration. The parameters (such as `logs_dd_url` can be
  either be specified in the `compliance_config.endpoints` section or through
  environment variables (such as DD_COMPLIANCE_CONFIG_ENDPOINTS_LOGS_DD_URL).

- Improve the resilience of the connection of controllers to the External Metrics Server by moving to a dynamic client for the WPA controller.


.. _Release Notes_dca-1.12.0_dca-1.12.X_Upgrade Notes:

Upgrade Notes
-------------

- Change base Docker image used to build the Cluster Agent imges, moving from debian:bullseye to ubuntu:20.10.
  In the future the Cluster Agent will follow Ubuntu stable versions.


.. _Release Notes_dca-1.12.0_dca-1.12.X_Bug Fixes:

Bug Fixes
---------

- Fix a potential file descriptors leak.

- The Cluster Agent can now be configured to use tls 1.2 via DD_FORCE_TLS_12=true

- Fix "Error creating expvar server" error log when running the Datadog Cluster Agent CLI commands.

- Fix a bug preventing the
  "DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_ADDITIONAL_ENDPOINTS" environment
  variable to be read.


.. _Release Notes_dca-1.11.0_dca-1.11.X:

dca-1.11.0
==========

.. _Release Notes_dca-1.11.0_dca-1.11.X_Prelude:

Prelude
-------

Released on: 2021-03-02
Pinned to datadog-agent v7.26.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7260--6260>`_.


.. _Release Notes_dca-1.11.0_dca-1.11.X_New Features:

New Features
------------

- Support Prometheus Autodiscovery for Kubernetes Services.


.. _Release Notes_dca-1.11.0_dca-1.11.X_Enhancement Notes:

Enhancement Notes
-----------------

- Add `external_metrics_provider.api_key` and `external_metrics_provider.app_key` parameters overriding default `api_key` and `app_key` if set.

- Add a new external_metrics_provider.endpoint config in datadog-cluster.yaml
  and a DD_EXTERNAL_METRICS_PROVIDER_ENDPOINT environment variable to
  override the default Datadog API endpoint to query external metrics from,
  in place of the global DATADOG_HOST. It also makes the external metrics
  provider respect DD_SITE if DD_EXTERNAL_METRICS_PROVIDER_ENDPOINT is not
  set.

- Node schedulability is now a dedicated tag on kubernetes node resources.


.. _Release Notes_dca-1.11.0_dca-1.11.X_Bug Fixes:

Bug Fixes
---------

- Fix dual shipping for orchestrator resources in the cluster agent.


.. _Release Notes_dca-1.10.0_dca-1.10.X:

1.10.0
==========

Prelude
-------

Released on: 2021-03-02
    Pinned to datadog-agent v7.24.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7240--6240>`_..

.. _Release Notes_dca-1.10.0_dca-1.10.X_New Features:

New Features
------------

- Add a new command 'datadog-cluster-agent health' to show the cluster
  agent's health, similar to the already existing `agent health`.

- collect node information for the orchestrator explorer

- Fill DatadogMetric `AutoscalerReferences` field to ease usage/investigation of DatadogMetrics

- The Cluster Agent can now collect stats from Cluster Level Check runners
  to optimize its dispatching logic and rebalance the scheduled checks.

- Allow providing custom tags to orchestrator resources.


.. _Release Notes_dca-1.10.0_dca-1.10.X_Enhancement Notes:

Enhancement Notes
-----------------

- Add new configuration parameter to allow 'GroupExec' permission on the secret-backend command.
  The new parameter ('secret_backend_command_allow_group_exec_perm') is now enabled by default in the cluster-agent image.

- Add resolve option to endpoint checks through new annotation `ad.datadoghq.com/endpoints.resolve`. With `ip` value, it allows endpoint checks to target static pods

- Expose metrics for the cluster level checks advanced dispatching.


.. _Release Notes_dca-1.10.0_dca-1.10.X_Bug Fixes:

Bug Fixes
---------

- Fix 'readsecret.sh' permission in Cluster-Agent dockerfiles that removes `other` permission.

- Fix issue in Cluster Agent when using external metrics without DatadogMetrics where multiple HPAs using the same metricName + Labels would prevent all HPAs (except 1st one) to get values from Datadog

- Ensure that leader election runs if orchestrator_explorer and leader_election are enabled.

- Rename node role tag from "node_role" to "kube_node_role" in orchestrator_explorer collection.


.. _Release Notes_dca-1.9.1_dca-1.9.x:

1.9.1
=====

.. _Release Notes_dca-1.9.1_dca-1.9.x_Prelude:

Prelude
-------

Released on: 2020-10-21
Pinned to datadog-agent v7.23.1: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7231>`_..

.. _Release Notes_dca-1.9.1_dca-1.9.x_Bug Fixes:

Bug Fixes
---------

- Support of secrets in JSON environment variables, added in `7.23.0`, is
  reverted due to a side effect (e.g. a string value of `"-"` would be loaded as a list). This
  feature will be fixed and added again in a future release.


.. _Release Notes_1.9.0:

1.9.0
=====

.. _Release Notes_1.9.0_Prelude:

Prelude
-------

Released on: 2020-10-13
Pinned to datadog-agent v7.23.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7230--6230>`_..

New Features
------------

- Collect the node and cluster resource in Kubernetes for the Orchestrator Explorer (#6297).
- Add `resolve` option to the endpoint checks (#5918).
- Add `health` command (#6144).
- Add options to configure the External Metrics Server (#6406).

Enhancement Notes
-----------------

- Fill DatadogMetric `AutoscalerReferences` field to ease usage/investigation of DatadogMetrics (#6367).
- Only run compliance checks on the Cluster Agent leader (#6311).
- Add `orchestrator_explorer` configuration to enable the cluster-id ConfigMap creation and Orchestrator Explorer instanciation (#6189).

Bug Fixes
---------

- Fix transformer for gibiBytes and gigaBytes (#6437).
- Fix `cluster-agent` commands to allow executing the `readsecret.sh` script for the secret backend feature (#6445).
- Fix issue with External Metrics when several HPAs use the same query (#6412).

.. _Release Notes_1.8.0:

1.8.0
=====

.. _Release Notes_1.8.0_Prelude:

Prelude
-------

Released on: 2020-08-07

New Features
------------

- Add compliance check command to the DCA CLI (#5930)
- Add `clusterchecks rebalance` command (#5839)
- Add collection of additional Kubernetes resource types (deployments, replicaSets and services) for Live Containers (#6082, #5999)


Enhancement Notes
-----------------

- Support "ignore AD tags" parameter for cluster/endpoint checks (#6115)
- Use APIserver connection retrier (#6106)

.. _Release Notes_1.7.0:

1.7.0
=====

.. _Release Notes_1.7.0_Prelude:

Prelude
-------

Released on: 2020-07-20

This version contains the changes released with version 7.21.0 of the core agent.
Please refer to the `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7210--6210>`_.

New Features
------------

- Add support of DatadogMetric CRD to allow autoscaling based on arbitrary queries (#5384)
- Add Admission Controller to inject Entity ID, standard tags and agent host (useful in serverless environments)

Enhancement Notes
-----------------

- Add `leader_election_is_leader` metric to allow label joins (#5819)


.. _Release Notes_1.6.0:

1.6.0
=====

.. _Release Notes_1.6.0_Prelude:

Prelude
-------

Released on: 2020-06-11

This version contains the changes released with version 7.20.0 of the core agent.
Please refer to the `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7200--6200>`_.

Bug Fixes
---------

- Wait for client-go cache to sync for endpoints/services (#5291)
- Consider check failure in advanced rebalancing (#5441)

New Features
------------

- Autodiscover standard tags for Cluster and Endpoint Checks (#5241)

Enhancement Notes
-----------------

- Adds a metric to monitor the advanced dispatching algorithm (#4970)

.. _Release Notes_1.5.2:

1.5.2
=====

.. _Release Notes_1.5.2_Prelude:

Prelude
-------

Released on: 2020-02-11

Minor release on 1.5 branch

Bug Fixed
------------

- Fix agent commands in DCA (always start listener) (#4870)

.. _Release Notes_1.5.1:

1.5.1
=====

.. _Release Notes_1.5.1_Prelude:

Prelude
-------

Released on: 2020-02-06

Minor release on 1.5 branch

Bug Fixed
------------

- [DCA] fix cluster-agent flare panic (#4838)
- Remove setcap NET_BIND_SERVICE as we cannot make it work with user namespaces used in the CI (#4846)
- Add service listener in endpoints to watch for newly annotated services (#4816)
- Fix typo (#4831)

.. _Release Notes_1.5.0:

1.5.0
=====

.. _Release Notes_1.5.0_Prelude:

Prelude
-------

Released on: 2020-01-28

This version contains the changes released with version 7.17.0 of the core agent.
Please refer to the `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#7170>`_.

New Features
------------

- Adding logic to show DCA status for clc (#4738)
- Introduce Rate Limiting Stats in the /metrics of the Cluster Agent (#4669)
- MetricServer generates k8s event on HPA

Enhancement Notes
-----------------

- Add cluster-name tag in host tags (#4558)
- Add read-secret command in cluster-agent to use as secrets backend (#4639)
- Adding logic to show DCA status for clc (#4738)
- Allow dots in cluster names (#4611)
- Check if CheckMetadata exist before iterating over it in cluster agent status page (#4728)
- Grant CAP_NET_BIND_SERVICE capability to the cluster_agent (#4439)
- Ignore invalid cluster names instead of panicking (#4549)
- Fix eventrecorder init (#4732)
- Handle NewHandler failure better in setupClusterCheck (#4447)
- Adding User-Agent to the DCA client
- Filter non-cluster-checks (#4566)

.. _Release Notes_1.4.0:

1.4.0
=====

.. _Release Notes_1.4.0_Prelude:

Prelude
-------

Released on: 2019-11-06

This version contains the changes released with version 6.15.0 of the core agent.
Please refer to the `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst#6150>`_.

New Features
------------

- Introducing the Advanced dispatching logic to rebalancing Cluster Level Checks [#4068, #4226, #4344]
- Enable the Endpoint check logic [#3853, #3704]
- HTTP proxy support for the external metrics provider #4191
- Improve External Metrics Provider resiliency [#4285, #3727]
- Revamp the Kubernetes event collection check [#4259, #4346, #4342, #4337, #4314]

Enhancement Notes
-----------------

- Update Gopkg.lock with new import #3837
- Fix kubernetes_apiserver default config file #3854
- Fix registration of the External Metrics Server's API #4233
- Fixing status of the Cluster Agent if the External Metrics Provider is not enabled #4277
- Fix how the endpoints check source is displayed in agent command outputs #4357
- Fix how we invalidate changed Endpoints config #4363
- Get Cluster Level Checks runner IPs from headers #4386
- Fixing output of `agent status` #4352

1.3.2
=====
2019-07-09

- Fix Cluster-agent failure with `cluster-agent flare` command.

1.3.1
=====
2019-06-19

- Fix "Kube Services" service: `kube service` tags attached to pod are not consistent.

.. _Release Notes_1.3.0:

1.3.0
=====

.. _Release Notes_1.3.0_Prelude:

Prelude
-------

Released on: 2019-05-07

The Datadog Cluster Agent can now auto-discover config templates for kubernetes endpoints checks and expose them to node Agents via its API. This feature is compatible with the version 6.12.0 and up of the Datadog Agent.

Refer to `the official documentation <https://docs.datadoghq.com/agent/autodiscovery/endpointschecks/>`_ to read more about this feature.


1.3.0-rc.3
==========
2019-05-03

Bug Fixes
---------
- Fix race condition: immutable MetaBundle stored in DCA cache.

1.3.0-rc.2
==========
2019-04-30

Bug Fixes
---------
- Fix race condition in Cluster Agent's API handler.

1.3.0-rc.1
==========
2019-04-24

New Features
------------
- The Cluster Agent can now auto-discover config templates for kubernetes endpoints checks and expose them to node Agents via its API
- Add the ``config`` and ``configcheck`` command to the cluster agent CLI
- Add the ``diagnose`` command to the cluster agent CLI and flare
- Add cluster_checks.extra_tags option to allow users to add tags globally to the cluster level checks.

Enhancement Notes
-----------------
- Improving Lifecycle of the External Metrics Provider
- Support milliquantities for the External Metrics Provider
- Move some logs from info to debug, in order to generates fewer noisy logs when running correctly.

.. _Release Notes_1.2.0:

1.2.0
=====

.. _Release Notes_1.2.0_Prelude:

Prelude
-------

Released on: 2019-02-25

The Datadog Agent now supports distributing Cluster Level Checks. This feature is compatible with the version 6.9.0 and up of the Datadog Agent.

Refer to `the official documentation <https://docs.datadoghq.com/agent/autodiscovery/clusterchecks/>`_ to read more about this feature.

1.2.0-rc.5
==========
2019-02-14

Bug Fixes
---------
- Ensure dangling cluster checks can be re-scheduled

1.2.0-rc.4
==========
2019-02-12

Bug Fixes
---------
- Fix re-scheduling of the same clusterchecks config on the same node

1.2.0-rc.3
==========
2019-02-11

Enhancement Notes
-----------------
- Sign docker images when pushing to Docker Hub

Bug Fixes
---------
- Fix configcheck verbose output
- Fix AutoDiscovery rescheduling issue when no template variables
- Remove resolved configs when template are removed
- Support adding/removing the AD annotation to an existing kube service
- Only expose cluster-check prometheus metrics when leading
- Fix support for custom metrics case sensitivity

1.2.0-rc.2
==========
2019-02-05

Enhancement Notes
-----------------
The External Metrics Provider is now agnostic of the case, both on the metric name and the labels extracted from HPAs.

Bug Fixes
---------
- Cluster Agent HPA metrics case support

New Features
------------
- Add GetLeaderIP method to LeaderEngine
- Add kube_service config provider
- Allow to set additional Autodiscovery sources by envvars
- Add dispatching metrics in clusterchecks module
- Add a health probe in the ccheck dispatching logic
- Add kube-services AD listener
- Cluster-checks: handle leader election and follower->leader redirection
- Enable clusterchecks in DCA master
- Support /conf.d in cluster-agent image
- Fix clustercheck leader not starting its dispatching logic
- Use the appropriate port when redirecting node-agents to leader
- Cluster-checks: patch configurations on schedule
- Add configcheck/config cmd on the cluster agent
- Add clustercheck info to the cluster-agent's status and flare
- Make error in clusterchecks cmd clear when feature is disabled

1.2.0-rc.1
==========
2019-01-31

Note
----
The release of the RC1 was dismissed to embed a fix for the CI runners used to build the image.
- Go 1.11.5 compliancy + 1.11.5 for every CI
The official release of the Datadog Cluster Agent 1.2.0 starts with the RC2.

.. _Release Notes_1.1.0:

1.1.0
=====

.. _Release Notes_1.1.0_Prelude:

Prelude
-------

The version 1.1.0 of the Cluster Agent introduces new features and enhancements around the External Metrics Provider.

1.1.0-rc.2
==========
2018-11-21

Bug Fixes
---------
- Get goautoneg from github
- Fix datadog external metric query when no label is set

1.1.0-rc.1
==========
2018-11-20

Enhancement Notes
-----------------
- Migrating back to official custom metrics lib
- Change test to remove flakiness

New Features
------------
- Disable cluster checks in cluster-agent 1.1.x
- Allow users to change the custom metric provider port, to run as non-root
- Adding rollup and fix to circumvent time aggregation
- clusterchecks: simple dispatching logic
- Honor external metrics provider settings in cluster-agent status
- Run cluster-agent as non-root, support read-only rootfs
- Only push cluster-agent-dev:master from master

Bug Fixes
---------
- Fix folder permissions on containerd
- Adding fix for edge case in external metrics
- Fix flare if can't access APIServer
- DCA: fix custom metrics server
- Avoid panicking for missing fields in HPA

.. _Release Notes_1.0.0:

1.0.0
=====

.. _Release Notes_1.0.0_Prelude:

Prelude
-------

Released on: 2018-10-18

The Datadog Cluster Agent is compatible with versions 6.5.1 and up of the Datadog Agent.

- Please refer to the `6.5.0 tag on datadog-agent  <https://github.com/DataDog/datadog-agent/releases/tag/6.5.0>`_ for the list of changes on the Datadog Agent.

It is only supported in containerized environments.

- Please find the image on `our Docker Hub <https://hub.docker.com/r/datadog/cluster-agent/tags/>`_.

1.0.0-rc.4
==========
2018-10-17

Enhancement Notes
-----------------
- Expose telemetry metrics with the Open Metrics format instead of expvar

Bug Fixes
---------
- add mutex logic and safe guards to avoid race condition in the Autoscalers Controller.

1.0.0-rc.3
==========
2018-10-15

Enhancement Notes
-----------------
- Leverage diff logic to only update the internal custom metrics store and Config Map with relevant changes.
- Better logging on the Autoscalers Controller

Bug Fixes
---------
- Make sure only the leader sync Autoscalers.
- Forget keys from the informer's queue to avoid borking the Autoscalers Controller.

1.0.0-rc.2
==========
2018-10-11

Enhancement Notes
-----------------

- Support `agent` and `datadog-cluster-agent` for the CLI of the Datadog Cluster Agent
- Retrieve hostname in GCE

1.0.0-rc.1
==========
2018-10-04

New Features
------------

- Implement the External Metrics Interface to allow for the Horizontal Pod Autoscalers to be based off of Datadog metrics.
- Use informers to be up to date with the Horizontal Pod Autoscalers object in the cluster.
- Implement the metadata mapper.
- Use informers to be up to date with the Endpoints and Nodes objects in the cluster.
- Serve cluster level metadata on an external endpoint, `kube_service` tag is available.
- Serve node labels as tags.
- Run the kube_apiserver check to collect events and run a service check against each component of the Control Plane.
- Implements the `flare`, `status` and `version` commands similar to the node agent.
