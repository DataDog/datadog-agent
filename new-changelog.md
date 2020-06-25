=============
Release Notes
=============

.. _Release Notes_7.20.0:

7.20.0
======

.. _Release Notes_7.20.0_New Features:

New Features
------------

- Pod and container tags autodiscovered via pod annotations
  now support multiple values for the same key.

- Install script creates `install_info` report

- Agent detects `install_info` report and sends it through Host metadata

- Adding logic to get standard `service` tag from Pod Metadata Labels.

- APM: A new endpoint was added which helps augment and forward profiles
  to Datadog's intake.

- APM: Information about APM is now included in the agent's status
  output (both in the GUI and in the 'agent status' command).

- Introducing the 'cloud_provider_metadata' option in the Agent configuration
  to restrict which cloud provider metadata endpoints will be queried.

- Add collector for Garden containers running applications in CloudFoundry environment
  to view them in the live container list and container map.

- JMXFetch (helper for JMX checks) is now restarted if it crashes on Windows.

- Add scaffold for security/compliance agent CLI.

- `container_exclude_metrics` and `container_include_metrics` can be used to filter metrics collection for autodiscovered containers.
  `container_exclude_logs` and `container_include_logs` can be used to filter logs collection for autodiscovered containers.

- Support SNMP autodiscovery via a new configuration listener, with new
  template variables.

- Support Tencent Cloud provider.


.. _Release Notes_7.20.0_Enhancement Notes:

Enhancement Notes
-----------------

- When installing the Agent using Chocolatey,
  information about the installation is saved for
  diagnostic and telemetry purposes.

- The Agent's flare now includes information about the method used
  to install the Agent.

- Ignore AKS pause containers hosted in the aksrepos.azurecr.io
  container registry.

- Add a new `device_name` tag on IOstats and disk checks.

- Windows installer can use the command line key `HOSTNAME_FQDN_ENABLED` to set the config value of `hostname_fqdn`.

- Add missing `device_name` tags on docker, containerd and network checks.
  Make series manage `device_name` tag if `device` is missing.

- Support custom tagging of docker container data via an autodiscovery "tags"
  label key.

- Windows Docker image is now based on Windows Server Nano instead of Windows Server Core.

- Improved performances in metric aggregation logic.
  Use 64 bits context keys instead of 128 bits in order to benefit from better
  performances using them as map keys (fast path methods) + better performances
  while computing the hash thanks to inlining.

- Count of DNS responses with error codes are tracked for each connection.

- Latency of successful and failed DNS queries are tracked for each connection.Queries that time out are also tracked separately.

- Enrich dogstatsd metrics with task_arn tag if
  DD_DOGSTATSD_TAG_CARDINALITY=orchestrator.

- More pause containers from `ecr`, `gcr` and `mcr` are excluded automatically by the Agent.

- Improve cluster name auto-detection on Azure AKS.

- APM: Improve connection reuse with HTTP keep-alive in
  trace agent.

- Increase default timeout to collect metadata from GCE endpoint.

- Use insertion sort in the aggregator context keys generator as it provides
  better performances than the selection sort. In cases where the insertion
  sort was already used, improved its threshold selecting between it and Go
  stdlib sort.

- Expose distinct endpoints for liveness and readiness probes.
  * The liveness probe (``/live``) fails in case of unrecoverable error that deserve
    an agent restart. (Ex.: goroutine deadlock or premature exit)
  * The readiness probe (``/ready``) fails in case of recoverable errors or errors
    for which an agent restart would be more nasty than useful
    (Ex.: the forwarder fails to connect to DataDog)

- Exclude automatically pause containers for OpenShift, EKS and AKS Windows

- Introduce `kube_cluster_name` and `ecs_cluster_name` tags in addition to `cluster_name`.
  Add the possibility to stop sending the `cluster_name` tag using the parameter `disable_cluster_name_tag_key` in Agent config.
  The Agent keeps sending `kube_cluster_name` and `ecs_cluster_name` tags regardless of `disable_cluster_name_tag_key`. 

- Configure additional process and orchestrator endpoints by environment variable.

- The process agent can be canfigured to collect containers
  from multiple sources (e.g kubelet and docker simultaneously).

- Upgrading the embedded Python 2 to the latest, and final, 2.7.18 release.

- Improve performance of system-probe conntracker.

- Throttle netlink socket on workloads with high connection churn.


.. _Release Notes_7.20.0_Known Issues:

Known Issues
------------

- APM: Fixes a problem where writer queues would be inexistent if memory
  and CPU limitations are disabled.


.. _Release Notes_7.20.0_Deprecation Notes:

Deprecation Notes
-----------------

- `container_exclude` replaces `ac_exclude`.
  `container_include` replaces `ac_include`.
  `ac_exclude` and `ac_include` will keep being supported but the Agent ignores them
  in favor of `container_exclude` and `container_include` if they're defined.


.. _Release Notes_7.20.0_Bug Fixes:

Bug Fixes
---------

- APM: The reported "payload_too_large" metric, which counts dropped payloads
  due to their size, was previously incorrect and showed larger than normal numbers.

- APM: Fix a small programming error causing the "superfluous response.WriteHeader call" warning.

- Fix panic in the dogstatsd standalone package when running in a containerized environment.

- Fixes missing container stats in ECS Fargate 1.4.0.

- Ensure Python checks are always garbage-collected after they're unscheduled
  by AutoDiscovery.

- Fix for autodiscovered checks not being rescheduled after container restart.

- Fix S6 behavior when the core agent dies.
  When the core agent died in a multi-process agent container managed by S6,
  the container stayed in an unhealthy half dead state.
  S6 configuration has been modified so that it will now exit in case of
  core agent death so that the whole container will exit and will be restarted.

- On Windows, fix calculation of the ``system.swap.pct_free`` metric.

- Fix a bug in the file tailer on Windows where the log-agent would keep a
  lock on the file preventing users from renaming the it.


.. _Release Notes_7.20.0_Other Notes:

Other Notes
-----------

- Upgrade embedded ntplib to ``0.3.4``

- JMXFetch upgraded to `0.36.2 <https://github.com/DataDog/jmxfetch/releases/0.36.2>`_

- Rebranded puppy agent as iot-agent.


