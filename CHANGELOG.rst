=============
Release Notes
=============

.. _Release Notes_7.43.2:

7.43.2 / 6.43.2
======

.. _Release Notes_7.43.2_Prelude:

Prelude
-------

Release on: 2023-04-20


.. _Release Notes_7.43.2_Enhancement Notes:

Enhancement Notes
-----------------

- Upgraded JMXFetch to ``0.47.8`` which has improvements aimed
  to help large metric collections drop fewer payloads.


.. _Release Notes_7.43.1:

7.43.1 / 6.43.1
======

.. _Release Notes_7.43.1_Prelude:

Prelude
-------

Release on: 2023-03-07

- Please refer to the `7.43.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7431>`_ for the list of changes on the Core Checks.


.. _Release Notes_7.43.1_Enhancement Notes:

Enhancement Notes
-----------------

- Agents are now built with Go ``1.19.6``.


.. _Release Notes_7.43.0:

7.43.0 / 6.43.0
======

.. _Release Notes_7.43.0_Prelude:

Prelude
-------

Release on: 2023-02-23

- Please refer to the `7.43.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7430>`_ for the list of changes on the Core Checks


.. _Release Notes_7.43.0_Upgrade Notes:

Upgrade Notes
-------------

- The command line arguments to the Datadog Agent Manager for Windows ``ddtray.exe``
  have changed from single-dash arguments to double-dash arguments.
  For example, ``-launch-gui`` must now be provided as ``--launch-gui``.
  The start menu shortcut created by the installer will be automatically updated.
  Any custom scripts or shortcuts that launch ``ddtray.exe`` with arguments must be updated manually.


.. _Release Notes_7.43.0_New Features:

New Features
------------

- NDM: Add snmp.device.reachable/unreachable metrics to all monitored devices.

- Add a new ``container_image`` long running check to collect information about container images.

- Enable orchestrator manifest collection by default

- Add a new ``sbom`` core check to collect the software bill of materials of containers.

- The Agent now leverages DMI (Desktop Management Interface) information on Unix to get the instance ID on Amazon EC2 when the metadata endpoint fails or
is not accessible. The instance ID is exposed through DMI only on AWS Nitro instances.
This will not change the hostname of the Agent upon upgrading, but will add it to the list of host aliases.

- Adds the option to collect and store in workloadmeta the software bill of
  materials (SBOM) of containerd images using Trivy. This feature is disabled
  by default. It can be enabled by setting
  `container_image_collection.sbom.enabled` to true.
  Note: This feature is CPU and IO intensive.


.. _Release Notes_7.43.0_Enhancement Notes:

Enhancement Notes
-----------------

- Adds a new ``snmp.interface_status`` metric reflecting the same status as within NDM.

- APM: Ported a faster implementation of NormalizeTag with a fast-path for already normalized ASCII tags. Should marginally improve CPU usage of the trace-agent.

- The external metrics server now automatically adjusts the query time window based on the Datadog metrics `MaxAge` attribute.

- Added parity to Unix-based ``permissions.log`` Flare file on
  Windows. ``permissions.log`` file list the original rights/ACL
  of the files copied into a Agent flare. This will ease
  troubleshooting permissions issues.

- [corechecks/snmp] Add `id` and `source_type` to NDM Topology Links

- Add an ``--instance-filter`` option to the Agent check command.

- APM: Disable ``max_memory`` and ``max_cpu_percent`` by default in containerized environments (Docker-only, ECS and CI).
  Users rely on the orchestrator / container runtime to set resource limits.
  Note: ``max_memory`` and ``max_cpu_percent`` have been disabled by default in Kubernetes environments since Agent ``7.18.0``.

- Agents are now built with Go ``1.19.5``.

- To reduce "cluster-agent" memory consomption when `cluster_agent.collect_kubernetes_tags`
  option is enabled, we introduce `cluster_agent.kubernetes_resources_collection.pod_annotations_exclude` option
  to exclude Pod annotation from the extracted Pod metadata.

- Introduce a new option `enabled_rfc1123_compliant_cluster_name_tag`
  that enforces the `kube_cluster_name` tag value to be
  an RFC1123 compliant cluster name. It can be disabled by setting this
  new option to `false`.

- Allows profiling for the Process Agent to be dynamically enabled from the CLI with `process-agent config set internal_profiling`. Optionally, once profiling is enabled, block, mutex, and goroutine profiling can also be enabled with `process-agent config set runtime_block_profile_rate`, `process-agent config set runtime_mutex_profile_fraction`, and `process-agent config set internal_profiling_goroutines`.

- Adds a new process discovery hint in the process agent when the regular process and container checks run.

- Added new telemetry metrics (``pymem.*``) to track Python heap usage.

- There are two default config files. Optionally, you can provide override config files.
  The change in this release is that for both sets, if the first config is inaccessible, the security agent startup process fails. Previously, the security agent would continue to attempt to start up even if the first config file is inaccessible.
  To illustrate this, in the default case, the config files are datadog.yaml and security-agent.yaml, and in that order. If datadog.yaml is inaccessible, the security agent fails immediately. If you provide overrides, like foo.yaml and bar.yaml, the security agent fails immediately if foo.yaml is inaccessible.
  In both sets, if any additional config files are missing, the security agent continues to attempt to start up, with a log message about an inaccessible config file. This is not a change from previous behavior.

- [corechecks/snmp] Add IP Addresses to NDM Metadata interfaces

- [corechecks/snmp] Add LLDP remote device IP address.

- prometheus_scrape: Adds support for `tag_by_endpoint` and `collect_counters_with_distributions` in the `prometheus_scrape.checks[].configurations[]` items.

- The OTLP ingest endpoint now supports the same settings and protocols as the OpenTelemetry Collector OTLP receiver v0.68.0.


.. _Release Notes_7.43.0_Deprecation Notes:

Deprecation Notes
-----------------

- The command line arguments to the Datadog Agent Manager for Windows ``ddtray.exe``
  have changed from single-dash arguments to double-dash arguments.
  For example, ``-launch-gui`` must now be provided as ``--launch-gui``.

- system_probe_config.enable_go_tls_support is deprecated and replaced by service_monitoring_config.enable_go_tls_support.


.. _Release Notes_7.43.0_Security Notes:

Security Notes
--------------

- Some HTTP requests sent by the Datadog Agent to Datadog endpoints were including the Datadog API key in the query parameters (in the URL).
  This meant that the keys could potentially have been logged in various locations, for example, in a forward or a reverse proxy server logs the Agent connected to.
  We have updated all requests to not send the API key as a query parameter.
  Anyone who uses a proxy to connect the Agent to Datadog endpoints should make sure their proxy forwards all Datadog headers (patricularly ``DD-Api-Key``).
  Failure to not send all Datadog headers could cause payloads to be rejected by our endpoints.


.. _Release Notes_7.43.0_Bug Fixes:

Bug Fixes
---------

- The secret command now correctly displays the ACL on a path with spaces.

- APM: Lower default incoming trace payload limit to 25MB. This more closely aligns with the backend limit. Some users may see traces rejected by the Agent that the Agent would have previously accepted, but would have subsequently been rejected by the trace intake. The Agent limit can still be configured via `apm_config.max_payload_size`.

- APM: Fix the `trace-agent -info` command when remote configuration is enabled.

- APM: Fix parsing of SQL Server identifiers enclosed in square brackets.

- Remove files created by system-probe at uninstall time.

- Fix the `kubernetes_state_core` check so that the host alias name
  creation uses a normalized (RFC1123 compliant) cluster name.

- Fix an issue in Autodiscovery that could prevent Cluster Checks containing secrets (ENC[] syntax) to be unscheduled properly.

- Fix panic due to uninitialized Obfuscator logger

- On Windows, fixes bug in which HTTP connections were not properly accounted
  for when the client and server were the same host (loopback).

- The Openmetrics check is no longer scheduled for Kubernetes headless services.


.. _Release Notes_7.43.0_Other Notes:

Other Notes
-----------

- Upgrade of the cgosymbolizer dependency to use
  ``github.com/ianlancetaylor/cgosymbolizer``.

- The Datadog Agent Manager ``ddtray.exe`` now requires admin to launch.


.. _Release Notes_7.42.0:

7.42.0 / 6.42.0
======

.. _Release Notes_7.42.0_Prelude:

Prelude
-------

Release on: 2023-01-23

- Please refer to the `7.42.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7420>`_ for the list of changes on the Core Checks


.. _Release Notes_7.42.0_Upgrade Notes:

Upgrade Notes
-------------

- Downloading and installing official checks with `agent integration install`
  is no longer supported for Agent installations that do not include an embedded
  python3.
  

.. _Release Notes_7.42.0_New Features:

New Features
------------

- Adding the `kube_api_version` tag to all orchestrator resources.

- Kubernetes Pod events generated by the `kubernetes_apiserver` can now
  benefit from the new `cluster-tagger` component in the Cluster-Agent.

- APM OTLP: Added compatibility for the OpenTelemetry Collector's datadogprocessor to the OTLP Ingest.

- The CWS agent now supports rules on mount events.

- Adding a configuration option, ``exclude_ec2_tags``, to exclude EC2 instance tags from being converted into host
  tags.

- Adds detection for a process being executed directly from memory without the binary present on disk.

- Introducing agent sampling rates remote configuration.

- Adds support for ``secret_backend_command_sha256`` SHA for the ``secret_backend_command`` executable. If ``secret_backend_command_sha256`` is used,
  the following restrictions are in place:
  - Value specified in the ``secret_backend_command`` setting must be an absolute path.
  - Permissions for the ``datadog.yaml`` config file must disallow write access by users other than ``ddagentuser`` or ``Administrators`` on Windows or the user running the Agent on Linux and macOS.
  The agent will refuse to start if the actual SHA256 of the ``secret_backend_command`` executable is different from the one specified by ``secret_backend_command_sha256``.
  The ``secret_backend_command`` file is locked during verification of SHA256 and subsequent run of the secret backend executable.

- Collect network devices topology metadata.

- Add support for AWS Lambda Telemetry API

- Adds three new metrics collected by the Lambda Extension
  
  `aws.lambda.enhanced.response_latency`: Measures the elapsed time in milliseconds from when the invocation request is received to when the first byte of response is sent to the client.
  
  `aws.lambda.enhanced.response_duration`: Measures the elapsed time in milliseconds between sending the first byte of the response to the client and sending the last byte of the response to the client.
  
  `aws.lambda.enhancdd.produced_bytes`: Measures the number of bytes returned by a function.

- Create cold start span representing time and duration of initialization of an AWS Lambda function.


.. _Release Notes_7.42.0_Enhancement Notes:

Enhancement Notes
-----------------

- Adds both the `StartTime` and `ScheduledTime` properties in the collector for Kubernetes pods.

- Add an option (`hostname_trust_uts_namespace`) to force the Agent to trust the hostname value retrieved from non-root UTS namespaces (Linux only).

- Metrics from Giant Swarm pause containers are now excluded by default.

- Events emitted by the Helm check now have "Error" status when the release fails.

- Add an ``annotations_as_tags`` parameter to the kubernetes_state_core check to allow attaching Kubernetes annotations as Datadog tags in a similar way that the ``labels_as_tags`` parameter does.

- Adds the ``windows_counter_init_failure_limit`` option.
  This option limits the number of times a check will attempt to initialize
  a performance counter before ceasing attempts to initialize the counter.

- [netflow] Expose collector metrics (from goflow) as Datadog metrics

- [netflow] Add prometheus listener to expose goflow telemetry

- OTLP ingest now uses the minimum and maximum fields from delta OTLP Histograms and OTLP ExponentialHistograms when available.

- The OTLP ingest endpoint now reports the first cumulative monotonic sum value if the timeseries started after the Datadog Agent process started.

- Added the `workload-list` command to the process agent. It lists the entities stored in workloadmeta.

- Allows running secrets in the Process Agent on Windows by sandboxing
  ``secret_backend_command`` execution to the ``ddagentuser`` account used by the Core Agent service.

- Add `process_context` tag extraction based on a process's command line arguments for service monitoring.
  This feature is configured in the `system-probe.yaml` with the following configuration:
  `service_monitoring_config.process_service_inference.enabled`.

- Reduce the overhead of using Windows Performance Counters / PDH in checks.

- The OTLP ingest endpoint now supports the same settings and protocol as the OpenTelemetry Collector OTLP receiver v0.64.1

- The OTLP ingest endpoint now supports the same settings and protocols as the OpenTelemetry Collector OTLP receiver v0.66.0.


.. _Release Notes_7.42.0_Deprecation Notes:

Deprecation Notes
-----------------

- Removes the `install-service` Windows agent command.

- Removes the `remove-service` Windows agent command.


.. _Release Notes_7.42.0_Security Notes:

Security Notes
--------------

- Upgrade the wheel package to ``0.37.1`` for Python 2.

- Upgrade the wheel package to ``0.38.4`` for Python 3.


.. _Release Notes_7.42.0_Bug Fixes:

Bug Fixes
---------

- APM: Fix an issue where container tags weren't working because of overwriting an essential tag on spans.

- APM OTLP: Fix an issue where a span's local "peer.service" attribute would not override a resource attribute-level service.

- On Windows, fixes a bug in the NPM network driver which could cause
  a system crash (BSOD).

- Create only endpoints check from prometheus scrape configuration
  when `prometheus_scrape.service.endpoint` option is enabled.

- Fix how Kubernetes events forwarding detects the Node/Host. 
  * Previously Nodes' events were not always attached to the correct host.
  * Pods' events from "custom" controllers might still be not attached to
    a host if the controller doesn't set the host in the `source.host` event's field.

- APM: Fix SQL parsing of negative numbers and improve error message.

- Fix a potential panic when df outputs warnings or errors among its standard output.

- Fix a bug where a misconfig error does not show when `hidepid=invisible`

- The agent no longer wrongly resolves its hostname on ECS Fargate when
  requests to the Fargate API timeout.

- Metrics reported through OTLP ingest now have the interval property unset.

- Fix a PDH query handle leak that occurred when a counter failed to add to a query.

- Remove unused environment variables `DD_AGENT_PY` and `DD_AGENT_PY_ENV` from known environment variables in flare command.

- APM: Fix SQL obfuscator parsing of identifiers containing dollar signs.


.. _Release Notes_7.42.0_Other Notes:

Other Notes
-----------

- JMXFetch upgraded to `0.47.2 <https://github.com/DataDog/jmxfetch/releases/0.47.2>`_

- Bump embedded Python3 to `3.8.16`.


.. _Release Notes_7.41.1:

7.41.1 / 6.41.1
======

.. _Release Notes_7.41.1_Prelude:

Release on: 2022-12-21


.. _Release Notes_7.41.1_Enhancement Notes:

- Agents are now built with Go ``1.18.9``.


.. _Release Notes_7.41.0:

7.41.0 / 6.41.0
======

.. _Release Notes_7.41.0_Prelude:

Prelude
-------

Release on: 2022-12-09

- Please refer to the `7.41.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7410>`_ for the list of changes on the Core Checks


.. _Release Notes_7.41.0_Upgrade Notes:

Upgrade Notes
-------------

- Troubleshooting commands in the Agent CLI have been moved to the `diagnose` command. `troubleshooting metadata_v5`
  command is now `diagnose show-metadata v5` and `troubleshooting metadata_inventory` is `diagnose show-metadata inventory`.

- Journald launcher can now create multiple tailers on the same journal when 
  ``config_id`` is specified. This change enables multiple configs to operate 
  on the same journal which is useful for tagging different units. 
  Note: This may have an impact on CPU usage. 

- Upgrade tracer_agent debugger proxy to use logs intake API v2 
  for uploading snapshots 

- The Agent now defaults to TLS 1.2 instead of TLS 1.0. The ``force_tls_12`` configuration parameter has been removed since it's now the default behavior. To continue using TLS 1.0 or 1.1, you must set the ``min_tls_version`` configuration parameter to either `tlsv1.0` or `tlsv1.1`.


.. _Release Notes_7.41.0_New Features:

New Features
------------

- Added a required infrastructure to enable protocol classification for Network Performance Monitoring in the future.
  The protocol classification will allow us to label each connection with a L7 protocol.
  The features requires Linux kernel version 4.5 or greater.

- parse the snmp configuration from the agent and pass it to the integrated snmpwalk command in case the customer only provides an ip address

- The Agent can send its own configuration to Datadog to be displayed in the `Agent Configuration` section of the host
  detail panel. See https://docs.datadoghq.com/infrastructure/list/#agent-configuration for more information. The
  Agent configuration is scrubbed of any sensitive information and only contains configuration you’ve set using the
  configuration file or environment variables.

- Windows: Adds support for Windows Docker "Process Isolation" containers running on a Windows host.


.. _Release Notes_7.41.0_Enhancement Notes:

Enhancement Notes
-----------------

- APM: All spans can be sent through the error and rare samplers via custom feature flag `error_rare_sample_tracer_drop`. This can be useful if you want to run those samplers against traces that were not sampled by custom tracer sample rules. Note that even user manual drop spans may be kept if this feature flag is set.

- APM: The trace-agent will log failures to lookup CPU usage at error level instead of debug.

- Optionally poll Agent and Cluster Agent integration configuration files for changes after startup. This allows the Agent/Cluster Agent to pick up new
  integration configuration without a restart.
  This is enabled/disabled with the `autoconf_config_files_poll` boolean configuration variable.
  The polling interval is configured with the `autoconf_config_files_poll_interval` (default 60s).
  Note: Dynamic removal of logs configuration is currently not supported.

- Added telemetry for the "container-lifecycle" check.

- On Kubernetes, the "cluster name" can now be discovered by using
  the Node label `ad.datadoghq.com/cluster-name` or any other label
  key configured using to the configuration option:
  `kubernetes_node_label_as_cluster_name`

- Agents are now built with Go 1.18.8.

- Go PDH checks now all use the PdhAddEnglishCounter API to
  ensure proper localization support.

- Use the `windows_counter_refresh_interval` configuration option to limit
  how frequently the PDH object cache can be refreshed during counter
  initialization in golang. This replaces the previously hardcoded limit
  of 60 seconds.

- [netflow] Add disable port rollup config.

- The OTLP ingest endpoint now supports the same settings and protocol as the OpenTelemetry Collector OTLP receiver v0.61.0.

- The `disable_file_logging` setting is now respected in the process-agent.

- The `process-agent check [check-name]` command no longer outputs to the configured log file to reduce noise in the log file.

- Logs a warning when the process agent cannot read other processes due to misconfiguration.

- DogStatsD caches metric metadata for shorter periods of time,
  reducing memory usage when tags or metrics received are different
  across subsequent aggregation intervals.

- The ``agent`` CLI subcommands related to Windows services are now
  consistent in use of dashes in the command names (``install-service``,
  ``start-service``, and so on). The names without dashes are supported as
  aliases.

- The Agent now uses the V2 API to submit series data to the Datadog intake
  by default. This can be reverted by setting ``use_v2_api.series`` to
  false.


.. _Release Notes_7.41.0_Deprecation Notes:

Deprecation Notes
-----------------

- APM: The Rare Sampler is now disabled by default. If you wish to enable it explicitly you can set apm_config.enable_rare_sampler or DD_APM_ENABLE_RARE_SAMPLER to true.


.. _Release Notes_7.41.0_Bug Fixes:

Bug Fixes
---------

- APM: Don't include extra empty 'env' entries in sampling priority output shown by `agent status` command.

- APM: Fix panic when DD_PROMETHEUS_SCRAPE_CHECKS is set.

- APM: DogStatsD data can now be proxied through the "/dogstatsd/v1/proxy" endpoint
  and the new "/dogstatsd/v2/proxy" endpoint over UDS, with multiple payloads
  separated by newlines in a single request body.
  See https://docs.datadoghq.com/developers/dogstatsd#setup for configuration details.

- APM - remove extra error message from logs.

- Fixes an issue where cluster check metrics would be sometimes sent with the host tags.

- The containerd check no longer emits events related with pause containers when `exclude_pause_container` is set to `true`.

- Discard aberrant values (close to 18 EiB) in the ``container.memory.rss`` metric.

- Fix Cloud Foundry CAPI Metadata tags injection into application containers.

- Fix Trace Agent's CPU stats by reading correct PID in procfs

- Fix a potential panic when df outputs warnings or errors among its standard output.

- The OTLP ingest is now consistent with the Datadog exporter (v0.56+) when getting a hostname from OTLP resource attributes for metrics and traces.

- Make Agent write logs when SNMP trap listener starts and Agent
  receives invalid packets.

- Fixed a bug in the workloadmeta store. Subscribers that asked to receive
  only `unset` events mistakenly got `set` events on the first subscription for
  all the entities present in the store. This only affects the
  `container_lifecycle` check.

- Fix missing tags on the ``kubernetes_state.cronjob.complete`` service check.

- In ``kubernetes_state_core`` check, fix the `labels_as_tags` feature when the same Kubernetes label must be turned into different Datadog tags, depending on the resource:
  
     labels_as_tags:
       daemonset:
         first_owner: kube_daemonset_label_first_owner
       deployment:
         first_owner: kube_deployment_label_first_owner

- Normalize the EventID field in the output from the windowsevent log tailer.
  The type will now always be a string containing the event ID, the sometimes
  present qualifier value is retained in a new EventIDQualifier field.

- Fix an issue where the security agent would panic, sending on a close
  channel, if it received a signal when shutting down while all
  components were disabled.

- Fix tokenization of negative numeric values in the SQL obfuscator to remove extra characters prepended to the byte array.


.. _Release Notes_7.40.1:

7.40.1
======

.. _Release Notes_7.40.1_Prelude:

Prelude
-------

Release on: 2022-11-09

- Please refer to the `7.40.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7401>`_ for the list of changes on the Core Checks


.. _Release Notes_7.40.1_Enhancement Notes:

Enhancement Notes
-----------------

- Agents are now built with Go 1.18.8.


.. _Release Notes_7.40.1_Bug Fixes:

Bug Fixes
---------

- Fix log collection on Kubernetes distributions using ``cri-o`` like OpenShift, which
  began failing in 7.40.0.

.. _Release Notes_7.40.0:

7.40.0 / 6.40.0
======

.. _Release Notes_7.40.0_Prelude:

Prelude
-------

Release on: 2022-11-02

- Please refer to the ``7.40.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7400>``_ for the list of changes on the Core Checks


.. _Release Notes_7.40.0_Upgrade Notes:

Upgrade Notes
-------------

- Starting Agent 7.40, the Agent will fail to start when unable to determine hostname instead of silently using unrelevant hostname (usually, a container id).
  Hostname resolution is key to many features and failure to determine hostname means that the Agent is not configured properly.
  This change mostly affects Agents running in containerized environments as we cannot rely on OS hostname.

- Universal Service Monitoring now requires a Linux kernel version of 4.14 or greater.


.. _Release Notes_7.40.0_New Features:

New Features
------------

- The Agent RPM package now supports Amazon Linux 2022 and Fedora 30+ without requiring the installation of the additional ``libxcrypt-compat`` system package.

- Add support for CAPI metadata and DCA tags collection in PCF containers.

- Add a username and password dialog window to the Windows Installer

- APM: DogStatsD data can now be proxied through the "/dogstatsd/v1/proxy" endpoint
  over UDP. See https://docs.datadoghq.com/developers/dogstatsd#setup for configuration details.

- Cloud Workload Security now has Agent version constraints for Macros in SECL expressions.

- Added the "helm_values_as_tags" configuration option in the Helm check.  It
  allows users to collect helm values from a Helm release and use them as
  tags to attach to the metrics and events emitted by the Helm check.

- Enable the new DogStatsD no-aggregation pipeline, capable of processing metrics
  with timestamps.
  Set ``dogstatsd_no_aggregation_pipeline`` to ``false`` to disable it.

- Adds ability to identify the interpreter of a script inside a script via the shebang. Example rule would be ``exec.interpreter.file.name == ~"python*"``. This feature is currently limited to one layer of nested script. For example, a python script in a shell script will be caught, but a perl script inside a python script inside a shell script will not be caught.


.. _Release Notes_7.40.0_Enhancement Notes:

Enhancement Notes
-----------------

- JMXFetch now supports ZGC Cycles and ZGC Pauses beans support out of the box.

- Adds new ``aws.lambda.enhanced.post_runtime_duration`` metric for AWS Lambda
  functions. This gauge metric measures the elapsed milliseconds from when
  the function returns the response to when the extensions finishes. This
  includes performing activities like sending telemetry data to a preferred
  destination after the function's response is returned. Note that
  ``aws.lambda.enhanced.duration`` is equivalent to the sum of
  ``aws.lambda.enhanced.runtime_duration`` and
  ``aws.lambda.enhanced.post_runtime_duration``.

- Add the ``flare`` command to the Cloud Foundry ``cluster agent`` to improve support
  experience.

- Add ``CreateContainerError`` and ``InvalidImageName`` to waiting reasons
  for ``kubernetes_state.container.status_report.count.waiting`` in the Kubernetes State Core check.

- [netflow] Ephemeral Port Rollup

- APM: A warning is now logged when the agent is under heavy load.

- APM: The "http.status_code" tag is now supported as a numeric value too when computing APM trace stats. If set as both a string and a numeric value, the numeric value takes precedence and the string value is ignored.

- APM: Add support for cgroup2 via UDS.

- A new config option, ``logs_config.file_wildcard_selection_mode``, 
  allows you to configure how log wildcard file matches are
  prioritized if the number of matches exceeds ``logs_config.open_files_limit``.
  
  The option defaults to ``by_name`` which is the previous behavior.
  The new option is ``by_modification_time`` which prioritizes more recently
  modified files, but using it can result in slower performance compared to using ``by_name``.

- Agents are now built with Go 1.18.7.  This version of Go brings `changes to
  the garbage collection runtime <https://go.dev/doc/go1.18#runtime>`_ that
  may change the Agent's memory usage.  In internal testing, the RSS of Agent
  processes showed a minor increase of a few MiB, while CPU usage remained
  consistent.  Reducing the value of ``GOGC`` as described in the Go
  documentation was effective in reducing the memory usage at a modest cost
  in CPU usage.

- KSM Core check: Add the ``helm_chart`` tag automatically from the standard helm label ``helm.sh/chart``.

- Helm check: Add a ``helm_chart`` tag, equivalent to the standard helm label ``helm.sh/chart`` (see https://helm.sh/docs/chart_best_practices/labels/).

- The OTLP ingest endpoint now supports the same settings and protocol as the OpenTelemetry Collector OTLP receiver v0.60.0. In particular, this drops support for consuming OTLP/JSON v0.15.0 or below payloads.

- Improve CCCache performance on cache miss, significantly reducing
  the number of API calls to the CAPI.

- Add more flags to increase control over the CCCache, such as ``refresh_on_cache_miss``, ``sidecars_tags``,
  and ``isolation_segments_tags`` flags under ``cluster_agent`` properties.

- Windows: Add a config option to control how often the agent refreshes performance counters.

- Introduces an ``unbundle_events`` config to the ``docker`` integration. When
  set to ``true``, Docker events are no longer bundled together by image name,
  and instead generate separate Datadog events.

- Introduces an ``unbundle_events`` config to the ``kubernetes_apiserver``
  integration. When set to ``true``, Kubernetes events are no longer bundled
  together by InvolvedObject, and instead generate separate Datadog events.

- On Windows the Agent now uses high-resolution icon where possible.
  The smaller resolution icons have been resampled for better visibility.


.. _Release Notes_7.40.0_Known Issues:

Known Issues
------------

- APM: OTLP Ingest: resource attributes such as service.name are correctly picked up by spans.
- APM: The "/dogstatsd/v1/proxy" endpoint can only accept a single payload at a time. This will
  be fixed in the v2 endpoint which will split payloads by newline.


.. _Release Notes_7.40.0_Deprecation Notes:

Deprecation Notes
-----------------

- The following Windows Agent container versions are removed: 1909, 2004, and 20H2.


.. _Release Notes_7.40.0_Bug Fixes:

Bug Fixes
---------

- Add the device field to the ``MetricPayload`` to ensure the device 
  tag is properly handled by the backend. 

- APM: Revised support for tracer single span sampling. See datadog-agent/pull/13461.

- Fixed a problem that could trigger in the containerd collector when
  fetching containers from multiple namespaces.

- Fixed a crash when ``dogstatsd_metrics_stats_enable`` is true

- Fix a bug in Autodiscovery preventing the Agent to correctly schedule checks or logs configurations on newly created PODs during a StatefulSet rollout.

- The included ``aerospike`` Python package is now correctly built against
  the embedded OpenSSL and thus the Aerospike integration can be successfully
  used on RHEL/CentOS.

- Fix configresolver to continue parsing when a null value is found. 

- Fixed issue with CPU count on MacOS

- The container CPU limit that is reported by ``docker`` and ``container`` checks on ECS was not defaulting to the task limit when no CPU limit is set at container level.

- Fix potential panic when removing a service that the log agent is currently tailing.

- On SUSE, fixes the permissions declared in the package list of the RPM package.
  This was causing package conflicts between the datadog-agent package and other packages
  with files in ``/usr/lib/systemd/system``.

- Fixed a resource leak in the helm check.

- Fix golang performance counter initialization errors when counters
  are not available during agent/check init time.
  Checks now retry the counter initilization on each interval.

- [snmp] Cache snmp dynamic tags from devices


.. _Release Notes_7.40.0_Other Notes:

Other Notes
-----------

- JMXFetch upgraded to ``0.47.1 https://github.com/DataDog/jmxfetch/releases/0.47.1``

- The ``logs_config.cca_in_ad`` feature flag now defaults to true.  This
  selects updated codepaths in Autodiscovery and the Logs Agent.  No behavior
  change is expected.  Please report any behavior that is "fixed" by setting
  this flag to false.


.. _Release Notes_7.39.1:

7.39.1 / 6.39.1
======

.. _Release Notes_7.39.1_Prelude:

Prelude
-------

Release on: 2022-09-27


.. _Release Notes_7.39.1_Security Notes:

Security Notes
--------------

- Bump ``github.com/open-policy-agent/opa`` to `v0.43.1 <https://github.com/open-policy-agent/opa/releases/tag/v0.43.1>`_ to patch CVE-2022-36085.


.. _Release Notes_7.39.1_Other Notes:

Other Notes
-----------

- Bump embedded Python3 to `3.8.14`.

- Deactivated support of HTTP/2 in all non localhost endpoint used by Datadog Agent and Cluster Agent. (except endpoints)


.. _Release Notes_7.39.0:

7.39.0 / 6.39.0
======

.. _Release Notes_7.39.0_Prelude:

Prelude
-------

Release on: 2022-09-12

- Please refer to the `7.39.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7390>`_ for the list of changes on the Core Checks


.. _Release Notes_7.39.0_Upgrade Notes:

Upgrade Notes
-------------

- Starting with version 6.39.0, Agent 6 is no longer built for macOS.
  Only Agent 7 will be built for macOS going forward. macOS 10.14 and
  above are supported with Agent 7.39.0.


.. _Release Notes_7.39.0_New Features:

New Features
------------

- Add an integrated snmpwalk command to perform a walk for all snmp versions based on the gosnmp library.

- APM: Add two options under the `vector` config prefix to send traces
  to Vector instead of Datadog. Set `vector.traces.enabled` to true.
  Set `vector.traces.url` to point to a Vector endpoint. This overrides 
  the main endpoint. Additional endpoints remains fully functional.


.. _Release Notes_7.39.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add the `tagger-list` command to the `process-agent` to ease
  tagging issue investigation.

- Update SNMP traps database with bit enumerations.

- Resolve SNMP trap variables with bit enumerations to their string representation.

- Logs: Support filtering on arbitrary journal log fields

- APM: The trace-agent version string has been made more consistent and is now available in different build environments.

- Delay starting the auto multi-line detection timeout until at 
  least one log has been processed. 

- The ``helm`` check has new configuration parameters:
  - ``extra_sync_timeout_seconds`` (default 120)
  - ``informers_resync_interval_minutes`` (default 10)

- Improves the `labelsAsTags` feature of the Kubernetes State Metrics core check by performing the transformations of characters ['/' , '-' , '.'] 
  to underscores ['_'] within the Datadog agent.  
  Previously users had to perform these conversions manually in order to discover the labels on their resources.

- The new ``min_tls_version`` configuration parameter allows configuration of
  the minimum TLS version used for connections to the Datadog intake.  This
  replaces the ``force_tls_12`` configuration parameter which only allowed
  the minimum to be set to tlsv1.2.

- The OTLP ingest endpoint now supports the same settings and protocol as the OpenTelemetry Collector OTLP receiver v0.56.0

- 'agent status' command output is now parseable as JSON
  directly from stdout. Before this change, the
  logger front-matter made it hard to parse 'status'
  output directly as JSON.

- Raise the default ``logs_config.open_files_limit`` to ``200`` on 
  Windows and macOS. Raised to ``500`` for all other operating systems. 

- Support disabling DatadogMetric autogeneration with the
  external_metrics_provider.enable_datadogmetric_autogen configuration option
  (enabled by default).


.. _Release Notes_7.39.0_Deprecation Notes:

Deprecation Notes
-----------------

- APM: The `datadog.trace_agent.trace_writer.bytes_estimated` metric has been removed. It was meant to be a metric used for debugging, without any user added value.

- APM: The trace-agent /info endpoint no longer reports "build_date".

- The ``force_tls_12`` configuration parameter is deprecated, replaced by
  ``min_tls_version``.  If ``min_tls_version`` is not given, but ``force_tls_12``
  is true, then ``min_tls_version`` defaults to tlsv1.2.


.. _Release Notes_7.39.0_Bug Fixes:

Bug Fixes
---------

- Traps variable OIDs that had the index as a suffix are now correctly resolved.

- Agent status command should always log at info level to allow
  full status output regardless of Agent log level settings.

- APM: The "datadog.trace_agent.otlp.spans" metric was incorrectly reporting span count. This release fixes that.

- Fix panic when Agent stops jmxfetch.

- Fixed a bug in Kubernetes Autodiscovery based on pod annotations: The Agent no longer skips valid configurations if other invalid configurations exist.
  Note: This regression was introduced in Agents 7.36.0 and 6.36.0

- Fix a bug in autodiscovery that would not unschedule some checks when check configuration contains secrets.

- Orchestrator check: make sure we don't return labels and annotations with a suffixed `:`

- Fixed a bug in the Docker check that affects the
  `docker.containers.running` metric. It was reporting wrong values in cases
  where multiple containers with different `env`, `service`, `version`, etc.
  tags were using the same image.

- Fixed a deadlock in the DogStatsD when running the capture (`agent dogstatsd-capture`). The Agent now flushes the
  captured messages properly when the capture stops.

- Fix parsing of init_config in AD annotations v2.

- The ``internal_profiling.period`` parameter is now taken into account by the agent.

- Fix duplicated check or logs configurations, targeting dead containers when containers are re-created by Docker Compose.

- Fix concurrent map access issues when using OTLP ingest.

- [orchestrator check] Fixes race condition during check startup.

- The Windows installer will now respect the DDAGENTUSER_PASSWORD option and update the services passwords when the user already exists.

- The KSM Core check now handles cron job schedules with time zones.

- The v5 metadata payload's filesystem information is now more robust against failures in the ``df`` command, such as when a mountpoint is stuck.

- Fixes a disk check issue in the Docker Agent where a disproportionate amount of automount
  request system logs would be produced by the host after each disk check run.

- [epforwarder] Update NetFlow EP forwarder default configs

- The Agent starts faster on a Windows Docker host with many containers running by fetching the containers in parallel.

- On Windows, NPM driver adds support for Receive Segment Coalescing.
  This works around a Windows bug which in some situations causes
  system probe to hang on startup


.. _Release Notes_7.38.2:

7.38.2 / 6.38.2
======

.. _Release Notes_7.38.2_Prelude:

Prelude
-------

Release on: 2022-08-10

- Please refer to the `7.38.2 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7382>`_ for the list of changes on the Core Checks


.. _Release Notes_7.38.2_Bug Fixes:

Bug Fixes
---------

- Fixes a bug making the agent creating a lot of zombie (defunct) processes.
  This bug happened only with the docker images ``7.38.x`` when the containerized agent was launched without ``hostPID: true``.


.. _Release Notes_7.38.1:

7.38.1 / 6.38.1
======

.. _Release Notes_7.38.1_Prelude:

Prelude
-------

Release on: 2022-08-02


.. _Release Notes_7.38.1_Bug Fixes:

Bug Fixes
---------

- Fixes CWS rules with 'process.file.name !=""' expression.


.. _Release Notes_7.38.0:

7.38.0 / 6.38.0
======

.. _Release Notes_7.38.0_Prelude:

Prelude
-------

Release on: 2022-07-25

- Please refer to the `7.38.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7380>`_ for the list of changes on the Core Checks


.. _Release Notes_7.38.0_New Features:

New Features
------------


- Add NetFlow feature to listen to NetFlow traffic and forward them to Datadog.

- The CWS agent now supports filtering events depending on whether they are performed by a thread.
  A process is considered a thread if it's a child process that hasn't executed another program.

- Adds a `diagnose datadog-connectivity` command that displays information about connectivity issues between the Agent and Datadog intake.

- Adds support for tailing modes in the journald logs tailer.

- The CWS agent now supports writing rules on processes termination.

- Add support for new types of CI Visibility payloads to the Trace Agent, so
  features that until now were Agentless-only are available as well when using
  the Agent.


.. _Release Notes_7.38.0_Enhancement Notes:

Enhancement Notes
-----------------

- Tags configured with `DD_TAGS` or `DD_EXTRA_TAGS` in an EKS Fargate environment are now attached to OTLP metrics.

- Add NetFlow static enrichments (TCP flags, IP Protocol, EtherType, and more).

- Report lines matched by auto multiline detection as metrics
  and show on the status page. 

- Add a `containerd_exclude_namespaces` configuration option for the Agent to
  ignore containers from specific containerd namespaces.

- The `log_level` of the agent is now appended
  to the flare archive name upon its creation.

- The metrics reported by KSM core now include the tags "kube_app_name",
  "kube_app_instance", and so on, if they're related to a Kubernetes entity
  that has a standard label like "app.kubernetes.io/name",
  "app.kubernetes.io/instance", etc.

- The Kubernetes State Metrics Core check now collects two ingress metrics:
  ``kubernetes_state.ingress.count`` and ``kubernetes_state.ingress.path``.

- Move process chunking code to util package to avoid cycle import when using it in orchestrator check.

- APM: Add support for PostgreSQL JSON operators in the SQL obfuscate package.

- The OTLP ingest endpoint now supports the same settings and protocol as the OpenTelemetry Collector OTLP receiver v0.54.0 (OTLP v0.18.0).

- The Agent now embeds Python-3.8.13, an upgrade from
  Python-3.8.11.

- APM: Updated Rare Sampler default configuration values to sample traces more uniformly across environments and services.

- The OTLP ingest endpoint now supports Exponential Histograms with delta aggregation temporality.

- The Windows installer now supports grouped Managed Service Accounts.

- Enable https monitoring on arm64 with kernel >= 5.5.0.

- Add ``otlp_config.debug.loglevel`` to determine log level when the OTLP Agent receives metrics/traces for debugging use cases.


.. _Release Notes_7.38.0_Deprecation Notes:

Deprecation Notes
-----------------

- Deprecate``otlp_config.metrics.instrumentation_library_metadata_as_tags`` in 
  in favor of ``otlp_config.metrics.instrumentation_scope_metadata_as_tags``.


.. _Release Notes_7.38.0_Bug Fixes:

Bug Fixes
---------

- When ``enable_payloads.series`` or ``enable_payloads.sketches`` are set to 
  false, don't log the error ``Cannot append a metric in a closed buffered channel``.

- Restrict permissions for the entrypoint executables of the Dockerfiles.

- Revert `docker.mem.in_use` calculation to use RSS Memory instead of total memory.

- Add missing telemetry metrics for HTTP log bytes sent.

- Fix `panic` in `container`, `containerd`, and `docker` when container stats are temporarily not available

- Fix prometheus check Metrics parsing by not enforcing a list of strings.

- Fix potential deadlock when shutting down an Agent with a log TCP listener.

- APM: Fixed trace rare sampler's oversampling behavior. With this fix, the rare sampler will sample rare traces more accurately.

- Fix journald byte count on the status page. 

- APM: Fixes an issue where certain (#> and #>>) PostgreSQL JSON operators were
  being interpreted as comments and removed by the obfuscate package.

- Scrubs HTTP Bearer tokens out of log output

- Fixed the triggered "svType != tvType; key=containerd_namespace, st=[]interface
  {}, tt=[]string, sv=[], tv=[]" error when using a secret backend
  reader.

- Fixed an issue that made the container check to show an error in the "agent
  status" output when it was working properly but there were no containers
  deployed.


.. _Release Notes_7.37.1:

7.37.1 / 6.37.1
======

.. _Release Notes_7.37.1_Prelude:

Prelude
-------

Release on: 2022-06-28


.. _Release Notes_7.37.1_Bug Fixes:

Bug Fixes
---------

- Fixes issue where proxy config was ignored by the trace-agent.


.. _Release Notes_7.37.0:

7.37.0 / 6.37.0
======

.. _Release Notes_7.37.0_Prelude:

Prelude
-------

Release on: 2022-06-27

- Please refer to the `7.37.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7370>`_ for the list of changes on the Core Checks


.. _Release Notes_7.37.0_Upgrade Notes:

Upgrade Notes
-------------

- OTLP ingest: Support for the deprecated ``experimental.otlp`` section and the ``DD_OTLP_GRPC_PORT`` and ``DD_OTLP_HTTP_PORT`` environment variables has been removed. Use the ``otlp_config`` section or the ``DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT`` and ``DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT`` environment variables instead.

- OTLP: Deprecated settings ``otlp_config.metrics.report_quantiles`` and ``otlp_config.metrics.send_monotonic_counter`` have been removed in favor of ``otlp_config.metrics.summaries.mode`` and ``otlp_config.metrics.sums.cumulative_monotonic_mode`` respectively.


.. _Release Notes_7.37.0_New Features:

New Features
------------

- Adds User-level service unit filtering support for Journald log collection via ``include_user_units`` and ``exclude_user_units``.

- A wildcard (`*`) can be used in either `exclude_units` or `exclude_user_units` if only a particular type of Journald log is desired.

- A new `troubleshooting` section has been added to the Agent CLI. This section will hold helpers to understand the
  Agent behavior. For now, the section only has two command to print the different metadata payloads sent by the Agent
  (`v5` and `inventory`).

- APM: Incoming OTLP traces are now allowed to set their own sampling priority.

- Enable NPM NAT gateway lookup by default.

- Partial support of IPv6 on EKS clusters
  * Fix the kubelet client when the IP of the host is IPv6.
  * Fix the substitution of `%%host%%` patterns inside the auto-discovery annotations:
    If the concerned pod has an IPv6 and the `%%host%%` pattern appears inside an URL context, then the IPv6 is surrounded by square brackets.

- OTLP ingest now supports the same settings and protocol version as the OpenTelemetry Collector OTLP receiver v0.50.0.

- The Cloud Workload Security agent can now monitor and evaluate rules on bind syscall.

- [corechecks/snmp] add scale factor option to metric configurations

- Evaluate ``memory.usage`` metrics based on collected metrics.


.. _Release Notes_7.37.0_Enhancement Notes:

Enhancement Notes
-----------------

- APM: ``DD_APM_FILTER_TAGS_REQUIRE`` and ``DD_APM_FILTER_TAGS_REJECT`` can now be a literal JSON array.
  e.g. ``["someKey:someValue"]`` This allows for matching tag values with the space character in them.

- SNMP Traps are now sent to a dedicated intake via the epforwarder.

- Update SNMP traps database to include integer enumerations.

- The Agent now supports a single ``com.datadoghq.ad.checks`` label in Docker,
  containerd, and Podman containers. It merges the contents of the existing
  ``check_names``, ``init_configs`` (now optional), and ``instances`` annotations
  into a single JSON value.

- Add a new Agent telemetry metric ``autodiscovery_poll_duration`` (histogram)
  to monitor configuration poll duration in Autodiscovery.

- APM: Added ``/config/set`` endpoint in trace-agent to change configuration settings during runtime.
  Supports changing log level(log_level).

- APM: When the X-Datadog-Trace-Count contains an invalid value, an error will be issued.

- Upgrade to Docker client 20.10, reducing the duration of `docker` check on Windows (requires Docker >= 20.10 on the host).

- The Agent maintains scheduled cluster and endpoint checks when the Cluster Agent is unavailable.

- The Cluster Agent followers now forward queries to the Cluster Agent leaders themselves. This allows a reduction in the overall number of connections to the Cluster Agent and better spreads the load between leader and forwarders.

- The ``kube_namespace`` tag is now included in all metrics,
  events, and service checks generated by the Helm check.

- Include `install_info` to `version-history.json`

- Allow nightly builds install on non-prod repos

- Add a ``kubernetes_node_annotations_as_tags`` parameter to use Kubernetes node annotations as host tags.

- Add more detailed logging around leadership status failures.

- Move the experimental SNMP Traps Listener configuration under ``network_devices``.

- Add support for the DNS Monitoring feature of NPM to Linux kernels older than 4.1.

- Adds ``segment_name`` and ``segment_id`` tags to PCF containers that belong to an isolation segment.

- Make logs agent ``additional_endpoints`` reliable by default.
  This can be disabled by setting ``is_reliable: false``
  on the additional endpoint.

- On Windows, if a ``datadog.yaml`` file is found during an installation or
  upgrade, the dialogs collecting the API Key and Site are skipped.

- Resolve SNMP trap variables with integer enumerations to their string representation.

- [corechecks/snmp] Add profile ``static_tags`` config

- Report telemetry metrics about the retry queue capacity: ``datadog.agent.retry_queue_duration.capacity_secs``, ``datadog.agent.retry_queue_duration.bytes_per_sec`` and ``datadog.agent.retry_queue_duration.capacity_bytes``

- Updated cloud providers to add the Instance ID as a host alias
  for EC2 instances, matching what other cloud providers do. This
  should help with correctly identifying hosts where the customer
  has changed the hostname to be different from the Instance ID.

- NTP check: Include ``/etc/ntpd.conf`` and ``/etc/openntpd/ntpd.conf`` for ``use_local_defined_servers``.

- Kubernetes pod with short-lived containers do not have log lines duplicated with both container tags (the stopped one and the running one) when logs are collected.
  This feature is enabled by default, set ``logs_config.validate_pod_container_id`` to ``false`` to disable it.


.. _Release Notes_7.37.0_Security Notes:

Security Notes
--------------

- The Agent is built with Go 1.17.11.


.. _Release Notes_7.37.0_Bug Fixes:

Bug Fixes
---------

- Updates defaults for the port and binding host of the experimental traps listener.

- APM: The Agent is now performing rare span detection on all spans,
  as opposed to only dropped spans. This change will slightly reduce
  the number of rare spans kept unnecessarily.

- APM OTLP: This change ensures that the ingest now standardizes certain attribute keys to their correct Datadog tag counter parts, such as: container tags, "operation.name", "service.name", etc.

- APM: Fix a bug where the APM section of the GUI would not show up in older Internet Explorer versions on Windows.

- Support dynamic Auth Tokens in Kubernetes v1.22+ (Bound Service Account Token Volume).

- The "%%host%%" autodiscovery tag now works properly when using containerd, but only on Linux and when using IP v4 addresses.

- Enhanced the coverage of pause-containers filtering on Containerd.

- APM: Fix the loss of trace metric container information when large payloads need to be split.

- Fix `cri` check producing no metrics when running on `OpenShift / cri-o`.

- Fix missing health status from Docker containers in Live Container View.

- Fix Agent startup failure when running as a non-privileged user (for instance, when running on OpenShift with ``restricted`` SCC).

- Fix missing container metrics (container, containerd checks and live container view) on AWS Bottlerocket.

- APM: Fixed an issue where "CPU threshold exceeded" logs would show the wrong user CPU usage by a factor of 100.

- Ensures that when ``kubernetes_namespace_labels_as_tags`` is set, the namespace labels are always attached to metrics and logs, even when the pod is not ready yet.

- Add missing support for UDPv6 receive path to NPM.

- The ``agent workload-list --verbose`` command and the ``workload-list.log`` file in the flare
  do not show containers' environment variables anymore. Except for ``DD_SERVICE``, ``DD_ENV`` and ``DD_VERSION``.

- Fixed a potential deadlock in the Python check runner during agent shutdown.

- Fixes issue where trace-agent would not report any version info.

- The DCA and the cluster runners no longer write warning logs to `/tmp`.

- Fixes an issue where the Agent would panic when trying to inspect Docker
  containers while the Docker daemon was unavailable or taking too long to
  respond.


.. _Release Notes_7.37.0_Other Notes:

Other Notes
-----------

- Exclude teradata on Mac agents.


.. _Release Notes_7.36.1:

7.36.1 / 6.36.1
======

.. _Release Notes_7.36.1_Prelude:

Prelude
-------

Release on: 2022-05-31

- Please refer to the `7.36.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7361>`_ for the list of changes on the Core Checks


.. _Release Notes_7.36.1_Bug Fixes:

Bug Fixes
---------

- Fixes issue where proxy config was ignored by the trace-agent.

- This fixes a regression introduced in ``7.36.0`` where some logs sources attached to a container/pod would not be
  unscheduled on container/pod stop if multiple logs configs were attached to the container/pod.
  This could lead to duplicate log entries being created on container/pod restart as there would
  be more than one tailer tailing the targeted source.


.. _Release Notes_7.36.0:

7.36.0 / 6.36.0
======

.. _Release Notes_7.36.0_Prelude:

Prelude
-------

Release on: 2022-05-24

- Please refer to the `7.36.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7360>`_ for the list of changes on the Core Checks


.. _Release Notes_7.36.0_Upgrade Notes:

Upgrade Notes
-------------

- Debian packages are now built on Debian 8. Newly built DEBs are supported
  on Debian >= 8 and Ubuntu >= 14.

- The OTLP endpoint will no longer enable the legacy OTLP/HTTP endpoint ``0.0.0.0:55681`` by default. To keep using the legacy endpoint, explicitly declare it via the ``otlp_config.receiver.protocols.http.endpoint`` configuration setting or its associated environment variable, ``DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT``.

- Package signing keys were rotated:
  
  * DEB packages are now signed with key ``AD9589B7``, a signing subkey of key `F14F620E <https://keys.datadoghq.com/DATADOG_APT_KEY_F14F620E.public>`_
  * RPM packages are now signed with key `FD4BF915 <https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public>`_


.. _Release Notes_7.36.0_New Features:

New Features
------------

- Adding support for IBM cloud. The agent will now detect that we're running on IBM cloud and collect host aliases
  (vm name and ID).

- Added event collection in the Helm check. The feature is disabled by default. To enable it, set the ``collect_events`` option to true.

- Adds a service check for the Helm check. The check fails for a release when its latest revision is in "failed" state.

- Adds a ``kube_qos`` (quality of service) tag to metrics associated with
  kubernetes pods and their containers.

- CWS can now track network devices creation and load TC classifiers dynamically.

- CWS can now track network namespaces.

- The DNS event type was added to CWS.

- The OTLP ingest endpoint is now considered GA for metrics.

.. _Release Notes_7.36.0_Enhancement Notes:

Enhancement Notes
-----------------

- Traps OIDs are now resolved to names using user-provided 'traps db' files in ``snmp.d/traps_db/``.

- The Agent now supports a single ``ad.datadoghq.com/$IDENTIFIER.checks``
  annotation in Kubernetes Pods and Services to configure Autodiscovery
  checks. It merges the contents of the existing "check_names",
  ``init_configs`` (now optional), and ``instances`` annotations into a single
  JSON value.

- ``DD_URL`` environment variable can now be used to set the Datadog intake URL just like ``DD_DD_URL``.
  If both ``DD_DD_URL`` and `DD_URL` are set, ``DD_DD_URL`` will be used to avoid breaking change.

- Added a ``process-agent version`` command, and made the output mimic the core agent.

- Windows: Add Datadog registry to Flare.

- Add ``--service`` flag to ``stream-logs`` command to filter
  streamed logs in detail.

- Support a simple date pattern for automatic multiline detection

- APM: The OTLP ingest stringification of non-standard Datadog values such as Arrays and KeyValues is now consistent with OpenTelemetry attribute stringification.

- APM: Connections to upload profiles to the Datadog intake are now closed
  after 47 seconds of idleness. Common tracer setups send one profile every
  60 seconds, which coincides with the intake's connection timeout and would
  occasionally lead to errors.

- The Cluster Agent now exposes a new metric ``cluster_checks_configs_info``.
  It exposes the node and the check ID as tags.

- KSM core check: add a new ``kubernetes_state.cronjob.complete``
  service check that returns the status of the most recent job for
  a cronjob.

- Retry more HTTP status codes for the logs agent HTTP destination. 

- ``COPYRIGHT-3rdparty.csv`` now contains each copyright statement exactly as it is shown on the original component.

- Adds ``sidecar_present`` and ``sidecar_count`` tags on Cloud Foundry containers
  that run apps with sidecar processes.

- Agent flare now includes output from the ``process`` and ``container`` checks.

- Add the ``--cfgpath`` parameter in the Process Agent replacing ``--config``.

- Add the ``check`` subcommand in the Process Agent replacing ``--check`` (``-check``).
  Only warn once if the ``-version`` flag is used.

- Adds human readable output of process and container data in the ``check`` command
  for the Process Agent.

- The Agent flare command now collects Process Agent performance profile data in the flare bundle when the ``--profile`` flag is used.


.. _Release Notes_7.36.0_Deprecation Notes:

Deprecation Notes
-----------------

- Deprecated ``process-agent --vesion`` in favor of ``process-agent version``.

- The logs configuration ``use_http`` and ``use_tcp`` flags have been deprecated in favor of ``force_use_http`` and ``force_use_tcp``.

- OTLP ingest: ``metrics.send_monotonic_counter`` has been deprecated in favor of ``metrics.sums.cumulative_monotonic_mode``. ``metrics.send_monotonic_counter`` will be removed in v7.37.

- OTLP ingest: ``metrics.report_quantiles`` has been deprecated in favor of ``metrics.summaries.mode``. ``metrics.report_quantiles`` will be removed in v7.37 / v6.37.

- Remove the unused ``--ddconfig`` (``-ddconfig``) parameter.
  Deprecate the ``--config`` (``-config``) parameter (show warning on usage).

- Deprecate the ``--check`` (``-check``) parameter (show warning on usage).


.. _Release Notes_7.36.0_Bug Fixes:

Bug Fixes
---------

- Bump GoSNMP to fix incomplete support of SNMP v3 INFORMs.

- APM: OTLP: Fixes an issue where attributes from different spans were merged leading to spans containing incorrect attributes.

- APM: OTLP: Fixed an inconsistency where the error message was left empty in cases where the "exception" event was not found. Now, the span status message is used as a fallback.

- Fixes an issue where some data coming from the Agent when running in ECS
  Fargate did not have ``task_*``, ``ecs_cluster_name``, ``region``, and
  ``availability_zone`` tags.

- Collect the "0" value for resourceRequirements if it has been set

- Fix a bug introduced in 7.33 that could prevent auto-discovery variable ``%%port_<name>%%`` to not be resolved properly.

- Fix a panic in the Docker check when a failure happens early (when listing containers)

- Fix missing ``docker.memory.limit`` (and ``docker.memory.in_use``) on Windows

- Fixes a conflict preventing NPM/USM and the TCP Queue Length check from being enabled at the same time.

- Fix permission of "/readsecret.sh" script in the agent Dockerfile when
  executing with dd-agent user (for cluster check runners)

- For Windows, fixes problem in upgrade wherein NPM driver is not automatically started by system probe.

- Fix Gohai not being able to fetch network information when running on a non-English windows (when the output of
  commands like ``ipconfig`` were not in English). ``gohai`` no longer relies on system commands but uses Golang ``net`` package
  instead (same as Linux hosts).
  This bug had the side effect of preventing network monitoring data to be linked back to the host.

- Time-based metrics (for example, ``kubernetes_state.pod.age``, ``kubernetes_state.pod.uptime``) are now comparable in the Kubernetes state core check.

- Fix a risk of panic when multiple KSM Core check instances run concurrently.

- For Windows, includes NPM driver 1.3.2, which has a fix for a BSOD on system probe shutdown.

- Adds new ``--json`` flag to ``check``. ``process-agent check --json`` now outputs valid json.

- On Windows, includes NPM driver update which fixes performance
  problem when host is under high connection load.

- Previously, the Agent could not log the start or end of a check properly after the first five check runs. The Agent now can log the start and end of a check correctly.


.. _Release Notes_7.36.0_Other Notes:

Other Notes
-----------

- Include pre-generated trap db file in the ``conf.d/snmp.d/traps_db/`` folder.

- Gohai dependency has been upgraded. This brings a newer version of gopsutil and a fix when fetching network
  information in non-english Windows (see ``fixes`` section).


.. _Release Notes_7.35.2:

7.35.2 / 6.35.2
======

.. _Release Notes_7.35.2_Prelude:

Prelude
-------

Release on: 2022-05-05

.. _Release Notes_7.35.2_Bug Fixes:

Bug Fixes
---------

- Fix a regression impacting CSPM metering

.. _Release Notes_7.35.1:

7.35.1 / 6.35.1
======

.. _Release Notes_7.35.1_Prelude:

Prelude
-------

Release on: 2022-04-12


.. _Release Notes_7.35.1_Bug Fixes:

Bug Fixes
---------

- The weak dependency of datadog-agent, datadog-iot-agent and dogstatsd deb
  packages on the datadog-signing-keys package has been fixed to ensure
  proper upgrade to version 1:1.1.0.


.. _Release Notes_7.35.0:

7.35.0 / 6.35.0
======

.. _Release Notes_7.35.0_Prelude:

Prelude
-------

Release on: 2022-04-07

- Please refer to the `7.35.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7350>`_ for the list of changes on the Core Checks


.. _Release Notes_7.35.0_Upgrade Notes:

Upgrade Notes
-------------

- Agent, Dogstatsd and IOT Agent RPMs now have proper preinstall dependencies.
  On AlmaLinux, Amazon Linux, CentOS, Fedora, RHEL and Rocky Linux, these are:
  
  - ``coreutils`` (provided by package ``coreutils-single`` on certain platforms)
  - ``grep``
  - ``glibc-common``
  - ``shadow-utils``
  
  On OpenSUSE and SUSE, these are:
  
  - ``coreutils``
  - ``grep``
  - ``glibc``
  - ``shadow``

- APM Breaking change: The `default head based sampling mechanism <https://docs.datadoghq.com/tracing/trace_ingestion/mechanisms?tab=environmentvariables#head-based-default-mechanism>`_
  settings `apm_config.max_traces_per_second` or `DD_APM_MAX_TPS`, when set to 0, will be sending 
  0% of traces to Datadog, instead of 100% in previous Agent versions. 

- The OTLP ingest endpoint is now considered stable for traces.
  Its configuration is located in the top-level `otlp_config section <https://github.com/DataDog/datadog-agent/blob/7.35.0/pkg/config/config_template.yaml#L2915-L2918>`_.
  
  Support for the deprecated ``experimental.otlp`` section and the ``DD_OTLP_GRPC_PORT`` and ``DD_OTLP_HTTP_PORT``
  environment variables will be removed in Agent 7.37. Use the ``otlp_config`` section or the
  ``DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT`` and ``DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT``
  environment variables instead.

- macOS 10.12 support has been removed. Only macOS 10.13 and later are now supported.


.. _Release Notes_7.35.0_New Features:

New Features
------------

- The Cloud Workload Security agent can now monitor and evaluate rules on signals (kill syscall).

- CWS allows to write SECL rule on environment variable values. 

- The security Agent now offers a command to directly download the policy file from the API.

- CWS: Policy can now define macros with items specified as a YAML list
  instead of a SECL expression, as:::
  
    - my_macro:
      values:
        - value1
        - value2
  
  In addition, macros and rules can now be updated in later loaded policies
  (``default.policy`` is loaded first, the other policies in the folder are loaded
  in alphabetical order).
  
  The previous macro can be modified with:::
  
    - my_macro:
      combine: merge
      values:
        - value3
  
  It can also be overriden with:::
  
    - my_macro:
      combine: override
      values:
        - my-single-value
  
  Rules can now also be disabled with:::
  
    - my_rule:
      disabled: true

- Cloud Workload Security now works on Google's Container Optimized OS LTS versions, starting
  from v81.

- CWS: Allow setting variables to store states through rule actions.
  Action rules can now be defined as follows:::
  
    - id: my_rule
      expression: ...
      actions:
        - set:
            name: my_boolean_variable
            value: true
        - set:
            name: my_string_variable
            value: a string
        - set:
            name: my_other_variable
            field: process.file.name
  
  These actions will be executed when the rule is triggered by an event.
  Right now, only ``set`` actions can be defined.
  ``name`` is the name of the variable that will be set by the actions.
  The value for the variable can be specified by using:

  - ``value`` for a predefined value
    (strings, integers, booleans, array of strings and array of integers are currently supported).
  - ``field`` for the value of an event field.
  
  Variable arrays can be modified by specifying ``append: true``.
  
  Variables can be reused in rule expressions like a regular variable:::

    - id: my_other_rule
      expression: |-
        open.file.path == ${my_other_variable}

  By default, variables are global. They can be bounded to a specific process by using the ``process``
  scope as follows:::

    - set:
        name: my_scoped_variable
        scope: process
        value: true
  
  The variable can be referenced in other expressions as ``${process.my_scoped_variable}``. When the process dies, the
  variable with be automatically freed.

- Configuration ``process_config.enabled`` is now split into two settings: ``process_config.process_collection.enabled`` and ``process_config.container_collection.enabled``. This will allow better control over the process Agent.
  ``process_config.enabled`` now translates to these new settings:

  * ``process_config.enabled=true``: ``process_config.process_collection.enabled=true``
  * ``process_config.enabled=false``: ``process_config.container_collection.enabled=true`` and ``process_config.process_collection.enabled=false``
  * ``process_config.enabled=disabled``: ``process_config.container_collection.enabled=false`` and ``process_config.process_collection.enabled=false``

- Expose additional CloudFoundry metadata in the DCA API that the
  PCF firehose nozzles can use to reduce the load on the CC API.

- Added new "Helm" cluster check that collects information about the Helm releases deployed in the cluster.

- Add the ``process_agent_runtime_config_dump.yaml`` file to the core Agent flare with ``process-agent`` runtime settings.

- Add ``process-agent status`` output to the core Agent status command.

- Added new ``process-agent status`` command to help with troubleshooting and for better consistency with the core Agent. This command is intended to eventually replace `process-agent --info`.

- CWS rules can now be written on kernel module loading and deletion events.

- The splice event type was added to CWS. It can be used to detect the Dirty Pipe vulnerability.

- Add two options under a new config prefix to send logs
  to Vector instead of Datadog. ``vector.logs.enabled``
  must be set to true, along with ``vector.logs.url`` that
  should be set to point to a Vector configured accordingly.
  This overrides the main endpoints, additional endpoints
  remains fully functional.

- Adds new Windows system check, winkmem.  This check reports the top users
  of paged and non-paged memory in the windows kernel.


.. _Release Notes_7.35.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add support for the device_namespace tag in SNMP Traps.

- SNMP Trap Listener now also supports protocol versions 1 and 3 on top of the existing v2 support.

- The cluster agent has an external metrics provider feature to allow using Datadog queries in Kubernetes HorizontalPodAutoscalers.
  It sometimes faces issues like:
  
    2022-01-01 01:01:01 UTC | CLUSTER | ERROR | (pkg/util/kubernetes/autoscalers/datadogexternal.go:79 in queryDatadogExternal) | Error while executing metric query ... truncated... API returned error: Query timed out
  
  To mitigate this problem, use the new ``external_metrics_provider.chunk_size`` parameter to reduce the number of queries that are batched by the Agent and sent together to Datadog.

- Added a new implementation of the `containerd` check based on the `container` check. Several metrics are not emitted anymore: `containerd.mem.current.max`, `containerd.mem.kernel.limit`, `containerd.mem.kernel.max`, `containerd.mem.kernel.failcnt`, `containerd.mem.swap.limit`, `containerd.mem.swap.max`, `containerd.mem.swap.failcnt`, `containerd.hugetlb.max`, `containerd.hugetlb.failcount`, `containerd.hugetlb.usage`, `containerd.mem.rsshuge`, `containerd.mem.dirty`, `containerd.blkio.merged_recursive`, `containerd.blkio.queued_recursive`, `containerd.blkio.sectors_recursive`, `containerd.blkio.service_recursive_bytes`, `containerd.blkio.time_recursive`, `containerd.blkio.serviced_recursive`, `containerd.blkio.wait_time_recursive`, `containerd.blkio.service_time_recursive`.
  The `containerd.image.size` now reports all images present on the host, container tags are removed.

- Migrate the cri check to generic check infrastructure. No changes expected in metrics.

- Tags configured with `DD_TAGS` or `DD_EXTRA_TAGS` in an ECS Fargate or EKS Fargate environment are now attached to Dogstatsd metrics.

- Added a new implementation of the `docker` check based on the `container` check. Metrics produced do not change. Added the capability to run the `docker` check on Linux without access to `/sys` or `/proc`, although with a limited number of metrics.

- The DogstatsD protocol now supports a new field that contains the client's container ID.
  This allows enriching DogstatsD metrics with container tags.

- When ``ec2_collect_tags`` is enabled, the Agent now attempts to fetch data
  from the instance metadata service, falling back to the existing
  EC2-API-based method of fetching tags.  Support for tags in the instance
  metadata service is an opt-in EC2 feature, so this functionality will
  not work automatically.

- Add support for ECS metadata v4 API
  https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-metadata-endpoint-v4.html

- Agents are now built with Go 1.17.6.

- On ECS Fargate and EKS Fargate, Agent-configured tags (``DD_TAGS``/``DD_EXTRA_TAGS``)
  are now applied to all integration-collected metrics.

- Logs from JMXFetch will now be included in the Agent logfile, regardless
  of the ``log_level`` setting of the Agent.

- Addition of two knobs to configure JMXFetch statsd client:
  
  * ``jmx_statsd_client_queue_size`` to set the client queue size.
  * ``jmx_statsd_telemetry_enabled`` to enable the client telemetry.

- KSMCore `node.ready` service check now reports `warning`
  instead of `unknown` when a node enters an unknown state.

- Added `DD_PROCESS_CONFIG_PROCESS_DD_URL` and `DD_PROCESS_AGENT_PROCESS_DD_URL` environment variables

- Added `DD_PROCESS_CONFIG_ADDITIONAL_ENDPOINTS` and `DD_PROCESS_AGENT_ADDITIONAL_ENDPOINTS` environment variables

- Automatically extract the ``org.opencontainers.image.source`` container label into the ``git.repository_url`` tag.

- The experimental OTLP ingest endpoint now supports the same settings as the OpenTelemetry Collector OTLP receiver v0.43.1.

- The OTLP ingest endpoint now supports the same settings as the OpenTelemetry Collector OTLP receiver v0.44.0.

- The OTLP ingest endpoint can now be configured through environment variables.

- The OTLP ingest endpoint now always maps conventional metric resource-level attributes to metric tags.

- OTLP ingest: the ``k8s.pod.uid`` and ``container.id`` semantic conventions
  are now used for enriching tags in OTLP metrics.

- Add the ``DD_PROCESS_CONFIG_MAX_PER_MESSAGE`` env variable to set the ``process_config.max_per_message``.
  Add the ``DD_PROCESS_CONFIG_MAX_CTR_PROCS_PER_MESSAGE`` env variable to set the ``process_config.max_ctr_procs_per_message``.

- Add the ``DD_PROCESS_CONFIG_EXPVAR_PORT`` and ``DD_PROCESS_AGENT_EXPVAR_PORT`` env variables to set the ``process_config.expvar_port``.
  Add the ``DD_PROCESS_CONFIG_CMD_PORT`` env variable to set the ``process_config.cmd_port``.

- Add the ``DD_PROCESS_CONFIG_INTERNAL_PROFILING_ENABLED`` env variable to set the ``process_config.internal_profiling.enabled``.

- Add the `DD_PROCESS_CONFIG_SCRUB_ARGS` and `DD_PROCESS_AGENT_SCRUB_ARGS` env variables to set the `process_config.scrub_args`.
  Add the `DD_PROCESS_CONFIG_CUSTOM_SENSITIVE_WORDS` and `DD_PROCESS_AGENT_CUSTOM_SENSITIVE_WORDS` env variables to set the `process_config.custom_sensitive_words`.
  Add the `DD_PROCESS_CONFIG_STRIP_PROC_ARGUMENTS` and `DD_PROCESS_AGENT_STRIP_PROC_ARGUMENTS` env variables to set the `process_config.strip_proc_arguments`.

- Added `DD_PROCESS_CONFIG_WINDOWS_USE_PERF_COUNTERS` and `DD_PROCESS_AGENT_WINDOWS_USE_PERF_COUNTERS` environment variables

- Add the ``DD_PROCESS_CONFIG_QUEUE_SIZE`` and ``DD_PROCESS_AGENT_QUEUE_SIZE`` env variables to set the ``process_config.queue_size``.
  Add the ``DD_PROCESS_CONFIG_RT_QUEUE_SIZE`` and ``DD_PROCESS_AGENT_RT_QUEUE_SIZE`` env variables to set the ``process_config.rt_queue_size``.
  Add the ``DD_PROCESS_CONFIG_PROCESS_QUEUE_BYTES`` and ``DD_PROCESS_AGENT_PROCESS_QUEUE_BYTES`` env variables to set the ``process_config.process_queue_bytes``.

- Changes process payload chunking in the process Agent to take into account
  the size of process details such as CLI and user name.
  Adds the process_config.max_message_bytes setting for the target max (uncompressed) payload size.

- When ``ec2_collect_tags`` is configured, the Agent retries API calls to gather EC2 tags before giving up.

- Retry HTTP transaction when the HTTP status code is 404 (Not found).

- Validate SNMP namespace to ensure it respects length and illegal character rules.

- Include `/etc/chrony.conf` for `use_local_defined_servers`.


.. _Release Notes_7.35.0_Deprecation Notes:

Deprecation Notes
-----------------

- The security Agent commands ``check-policies`` and ``reload`` are deprecated.
  Use ``runtime policy check`` and ``runtime policy reload`` respectively instead.

- Configuration ``process_config.enabled`` is now deprecated.  Use ``process_config.process_collection.enabled`` and ``process_config.container_collection.enabled`` settings instead to control container and process collection in the process Agent.

- Removed ``API_KEY`` environment variable from the process agent. Use ``DD_API_KEY`` instead

- Removes the ``DD_PROCESS_AGENT_CONTAINER_SOURCE`` environment variable from the Process Agent. The list of container sources now entirely depends on the activated features.

- Removed unused ``process_config.windows.args_refresh_interval`` config setting

- Removed unused ``process_config.windows.add_new_args`` config setting

- Removes the ``process_config.max_ctr_procs_per_message`` setting.


.. _Release Notes_7.35.0_Bug Fixes:

Bug Fixes
---------

- APM: OTLP: Fixes an issue where attributes from different spans were merged leading to spans containing incorrect attributes.

- APM: Fixed an issue which caused a panic when receiving OTLP traces with invalid data (specifically duplicate SpanIDs).

- Silence the misleading error message
  ``No valid api key found, reporting the forwarder as unhealthy``
  from the output of the ``agent check`` command.

- Fixed a deadlock in the Logs Agent.

- Exclude filters no longer apply to empty container names, images, or namespaces.

- Fix CPU limit calculation for Windows containers.

- Fix a rare panic in Gohai when collecting the system's Python version.

- For Windows, includes NPM driver 1.3.2, which has a fix for a BSOD on system probe shutdown.

- OTLP ingest now uses the exact sum and count values from OTLP Histograms when generating Datadog distributions.


.. _Release Notes_7.35.0_Other Notes:

Other Notes
-----------

- JMXFetch upgraded to `0.46.0` https://github.com/DataDog/jmxfetch/releases/0.46.0


.. _Release Notes_7.34.0:

7.34.0 / 6.34.0
======

.. _Release Notes_7.34.0_Prelude:

Prelude
-------

Release on: 2022-03-02

- Please refer to the `7.34.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7340>`_ for the list of changes on the Core Checks


.. _Release Notes_7.34.0_Upgrade Notes:

Upgrade Notes
-------------

- CWS uses `**` for subfolder matching instead of `*`.
  Previously, `*` was used to match files and subfolders. With this
  release, `*` will match only files and folders at the same level. Use`**`
  at the end of a path to match files and subfolders. `**` must be
  used at the end of the path. For example, the rule `open.file.path == "/etc/*"`
  has to be converted to `open.file.path == "/etc/**"`.

- `additional_endpoints` in the `logs_config` now uses the same compression
  configuration as the main endpoint when sending to HTTP destinations. Agents 
  that relied on using different compression settings for `additional_endpoints`
  may need to be reconfigured. 


.. _Release Notes_7.34.0_New Features:

New Features
------------

- Autodiscovery of integrations now works with Podman containers. The minimum
  Podman version supported is 3.0.0.

- Cloud provider detection now support Oracle Cloud. This includes cloud provider detection, host aliases and NTP
  servers.

- APM: Add proxy endpoint to allow Instrumentation Libraries to submit telemetry data.

- CWS now allows to write SECL rule based on process ancestor args.

- CWS now exposes the first argument of exec event. Usually the
  name of the executed program.

- Add a new `runtime reload` command to the `security-agent`
  to dynamically reload CWS policies.

- Enables process discovery check to run by default in the process agent.
  Process discovery is a lightweight process metadata collection check enabling
  users to see recommendations for integrations running in their environments.

- APM: Adds a new endpoint to the Datadog Agent to forward pipeline stats to the Datadog backend.

- The Cloud Workload Security agent can now monitor and evaluate rules on mmap, mprotect and ptrace.

- Add support for Shift JIS (Japanese) encoding.
  It should be manually enabled in a log configuration using
  ``encoding: shift-jis``.

- Extend SNMP profile syntax to support metadata definitions

- When running inside a container with the host `/etc` folder mounted to `/host/etc`, the agent will now report the
  distro informations of the host instead of the one from the container.

- Added telemetry for the workloadmeta store.


.. _Release Notes_7.34.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add Autodiscovery telemetry.

- APM: Add the option to collect SQL comments and commands during obfuscation.

- Adds the process_config.disable_realtime_checks config setting in the process
  Agent allowing users to disable realtime process and container checks. Note:
  This prevents refresh of stats in the Live Processes and Live Containers pages
  for processes and containers reported by the Agent.

- [corechecks/snmp] Add additional metadata fields

- Reduce the memory usage when flushing series.

- Specifying ``auto_multi_line_detection: false`` in an integration's
  ``logs_config`` will now disable detection for that integration, even if
  detection is enabled globally.

- Make ``agent checkconfig`` an alias of ``agent configcheck``

- Added possibility to watch all the namespaces when running on containerd
  outside Kubernetes. By default, the agent will report events and metrics
  from all the namespaces. In order to select a specific one, please set the
  `containerd_namespace` option.

- The container check now works for containers managed by runtimes that
  implement the CRI interface such as CRI-O.

- ``cri.*`` and ``container.*`` metrics can now be collected from the CRI API
  on Windows.

- When using ``site: ddog-gov.com``, the agent now uses Agent-version-based
  URLs and ``api.ddog-gov.com`` as it has previously done for other Datadog
  domains.

- Add telemetry for ECS queries.

- Agents are now built with Go 1.16.12.

- Add Kubelet queries telemetry.

- Add the ``kubernetes_node_annotations_as_host_aliases`` parameter to specify a list
  of Kubernetes node annotations that should be used as host aliases.
  If not set, it defaults to ``cluster.k8s.io/machine``.

- The experimental OTLP endpoint now supports the same settings as the OpenTelemetry Collector OTLP receiver v0.41.0.

- OTLP metrics tags are enriched when ``experimental.otlp.metrics.tag_cardinality`` is set to ``orchestrator``.
  This can also be controlled via the ``DD_OTLP_TAG_CARDINALITY`` environment variable.

- Make the Prometheus auto-discovery be able to schedule OpenMetrics V2 checks instead of legacy V1 ones.
  
  By default, the Prometheus annotations based auto-discovery will keep on scheduling openmetrics v1 check.
  But the agent now has a `prometheus_scrape.version` parameter that can be set to ``2`` to schedule the v2.
  
  The changes between the two versions of the check are described in
  https://datadoghq.dev/integrations-core/legacy/prometheus/#config-changes-between-versions

- Raised the max batch size of logs and events from `100` to `1000` elements. Improves
  performance in high volume scenarios. 

- Add saturation metrics for network and memory.

- The Agent no longer logs spurious warnings regarding proxy-related environment variables
  ``DD_PROXY_NO_PROXY``, ``DD_PROXY_HTTP``, and ``DD_PROXY_HTTPS``.

- [corechecks/snmp] Add agent host as tag when ``use_device_id_as_hostname`` is enabled.

- [corechecks/snmp] Add profile metadata match syntax

- [corechecks/snmp] Support multiple symbols for profile metadata

- On Windows, the installer now uses a zipped Python integration folder, which
  should result in faster install times.

- Add support for Windows 2022 in published Docker images


.. _Release Notes_7.34.0_Bug Fixes:

Bug Fixes
---------

- APM: Fix SQL obfuscation error on statements using bind variables starting with digits

- Adds Windows NPM driver 1.3.1, which contains a fix for the system crash on system-probe shutdown under heavy load.

- ``DD_CLUSTER_NAME`` can be used to define the ``kube_cluster_name`` on EKS Fargate.

- On Windows the Agent now correctly detects Windows 11.

- Fixes an issue where the Docker check would undercount the number of
  stopped containers in the `docker.containers.stopped` and
  `docker.containers.stopped.total` metrics, accompanied by a "Cannot split
  the image name" error in the logs.

- Fixed a bug that caused a panic when running the docker check in cases
  where there are containers stuck in the "Removal in Progress" state.

- On EKS Fargate, the `container` check is scheduled while no suitable metrics collector is available, leading to excessive logging. Also fixes an issue with Liveness/Readiness probes failing regularly.

- Allow Prometheus scrape `tls_verify` to be set to `false` and 
  change `label_to_hostname` type to `string`.

- Fixes truncated queries using temp tables in SQL Server.

- Fixes an NPM issue on Windows where if the first packet on a UDP flow
  is inbound, it is not counted correctly.

- On macOS, fix a bug where the Agent would not gracefully stop when sent a SIGTERM signal.

- Fix missing tags with eBPF checks (OOM Kill/TCP Queue Length) with some container runtimes (for instance, containerd 1.5).

- The experimental OTLP endpoint now ignores hostname attributes with localhost-like names for hostname resolution.

- Fixes an issue where cumulative-to-delta OTLP metrics conversion did not take the hostname into account.


.. _Release Notes_7.33.1:

7.33.1 / 6.33.1
======

.. _Release Notes_7.33.1_Prelude:

Prelude
-------

Release on: 2022-02-10


.. _Release Notes_7.33.1_Bug Fixes:

Bug Fixes
---------

- Fixes a panic that happens occasionally when handling tags for deleted
  containers or pods.

- Fixes security module failing to start on kernels 4.14 and 4.15.

.. _Release Notes_7.33.0:

7.33.0 / 6.33.0
======

.. _Release Notes_7.33.0_Prelude:

Prelude
-------

Release on: 2022-01-26

- Please refer to the `7.33.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7330>`_ for the list of changes on the Core Checks


.. _Release Notes_7.33.0_Upgrade Notes:

Upgrade Notes
-------------

- APM: The `apm_config.max_traces_per_second` setting no longer affects error sampling.
  To change the TPS for errors, use `apm_config.error_traces_per_second` instead.

- Starting from this version of the Agent, the Agent does not run on SLES 11. 
  The new minimum requirement is SLES >= 12 or OpenSUSE >= 15 (including OpenSUSE 42).

- Changed the default value of `logs_config.docker_container_use_file` to `true`.
  The agent will now prefer to use files for collecting docker logs and fall back
  to the docker socket when files are not available.

- Upgrade Docker base image to ubuntu:21.10 as new stable release.


.. _Release Notes_7.33.0_New Features:

New Features
------------

- Autodiscovery of integrations now works with containerd.

- Metadata information sent by the Agent are now part of the flares. This will allow for easier troubleshooting of
  issues related to metadata.

- APM: Added credit card obfuscation. It is off by default and can be enabled using the
  env. var. DD_APM_OBFUSCATION_CREDIT_CARDS_ENABLED or `apm_config.obfuscation.credit_cards.enabled`.
  There is also an option to enable an additional Luhn checksum check in order to eliminate
  false negatives, but it comes with a performance cost and should not be used unless absolutely
  needed. The option is DD_APM_OBFUSCATION_CREDIT_CARDS_LUHN or `apm_config.obfuscation.credit_cards.luhn`.

- APM: The rare sampler can now be disabled using the environment variable DD_APM_DISABLE_RARE_SAMPLER
  or the `apm_config.disable_rare_sampler` configuration. By default the rare sampler catches 5 extra trace chunks
  per second on top of the head base sampling.
  The TPS is spread to catch all combinations of service, name, resource, http.status, error.type missed by 
  head base sampling.

- APM: The error sampler TPS can be configured using the environment variable DD_APM_ERROR_TPS
  or the `apm_config.error_traces_per_second` configuration. It defaults to 10 extra trace chunks sampled 
  per second on top of the base head sampling.
  The TPS is spread to catch all combinations of service, name, resource, http.status, and error.type.

- Add a generic `container` check. It generates `container.*` metrics based on all running containers, regardless of the container runtime used (among the supported ones).

- Added new option "container_labels_as_tags" that allows the Agent to
  extract container label values and set them as metric tags values. It's
  equivalent to the existing "docker_labels_as_tags", but it also works with
  containerd.

- CSPM: enable the usage of the print function in Rego rules.

- CSPM: add option to dump reports to file, when running checks manually.
  CSPM: constants can now be defined in rego rules and will be usable from rego rules.

- CWS: SECL expressions can now make use of predefined variables.
  `${process.pid}` variable refers to the pid of the process that
  trigger the event. 

- Enable NPM DNS domain collection by default.

- Exposed additional *experimental* configuration for OTLP metrics
  translation via ``experimental.otlp.metrics``.

- Add two options under a new config prefix to send metrics
  to Vector instead of Datadog. `vector.metrics.enabled`
  must be set to true, along with `vector.metrics.url` that
  should be set to point to a Vector configured accordingly.

- The bpf syscall is now monitored by CWS; rules can be written on BPF commands.

- Add runtime settings support to the security-agent. Currenlty only the log-level
  is supported.

- APM: A new intake endpoint was added as /v0.6/traces, which accepts a new, more compact and efficient payload format.
  For more details, check: https://github.com/DataDog/datadog-agent/blob/7.33.0/pkg/trace/api/version.go#L78.


.. _Release Notes_7.33.0_Enhancement Notes:

Enhancement Notes
-----------------

- Adds Nomad namespace and datacenter to list of env vars extracted from Docker containers.

- Add a new `On-disk storage` section to `agent status` command.

- Run CSPM commands as a configurable user.
  Defaults to 'nobody'.

- CSPM: the findings query now defaults to `data.datadog.findings`

- The ``docker.exit`` service check has a new tag ``exit_code``.
  The ``143`` exit code is considered OK by default, in addition to ``0``.
  The Docker check supports a parameter ``ok_exit_codes`` to allow choosing exit codes that are considered OK.

- Allow dogstatsd replay files to be fully loaded into memory as opposed
  to relying on MMAP. We still default to MMAPing replay targets.

- ``kubernetes_state.node.*`` metrics are tagged with ``kubelet_version``,
  ``container_runtime_version``, ``kernel_version``, and ``os_image``.

- The Kube State Metrics Core check uses ksm v2.1.

- Lowercase the cluster names discovered from cloud providers
  to ease moving between different Datadog products.

- On Windows, allow enabling process discovery in the process agent by providing PROCESS_DISCOVERY_ENABLED=true to the msiexec command.

- Automatically extract the ``org.opencontainers.image.revision`` container label into the ``git.commit.sha`` tag.

- The experimental OTLP endpoint now can be configured through the ``experimental.otlp.receiver`` section and supports the same settings as the OpenTelemetry Collector OTLP receiver v0.38.0.

- The Process, APM, and Security agent now use the remote tagger introduced
  in Agent 7.26 by default. To disable it in the respective agent, the following
  settings need to be set to `false`:
  
  - apm_config.remote_tagger
  - process_config.remote_tagger
  - security_agent.remote_tagger

- Allows the remote tagger timeout at startup to be configured by setting the
  `remote_tagger_timeout_seconds` config value. It also now defaults to 30
  seconds instead of 5 minutes.

- Calls to cloud metadata APIs for metadata like hostnames and IP addresses
  are now cached and the existing values used when the metadata service
  returns an error.  This will prevent such metadata from temporarily
  "disappearing" from hosts.

- Datadog Process Agent Service is started automatically by the core agent on Windows when process discovery is enabled in the config.

- All packages - datadog-agent, datadog-iot-agent and datadog-dogstatsd -
  now support AlmaLinux and Rocky Linux distributions.

- If unrecognized ``DD_..`` environment variables are set, the agent will now log a warning at startup, to help catch deployment typos.

- Update the embedded ``pip`` version to 21.3.1 on Python 3 to
  allow the use of newer build backends.

- Metric series can now be submitted using the V2 API by setting
  `use_v2_api.series` to true.  This value defaults to false, and
  should only be set to true in internal testing scenarios.  The
  default will change in a future release.

- Add support for Windows 20H2 in published Docker images

- Add a new agent command to dump the content of the workloadmeta store ``agent workload-list``.
  The output of ``agent workload-list --verbose`` is included in the agent flare.


.. _Release Notes_7.33.0_Bug Fixes:

Bug Fixes
---------

- Strip special characters (\n, \r and \t) from OctetString

- APM: Fix bug where obfuscation fails for autovacuum sql text. 
  For example, SQL text like `autovacuum: VACUUM ANALYZE fake.table` will no longer fail obfuscation. 

- APM: Fix SQL obfuscation failures on queries with literals that include non alpha-numeric characters

- APM: Fix obfuscation error on SQL queries using the '!' operator.

- Fixed Windows Dockerfile scripts to make the ECS Fargate Python check run
  when the agent is deployed in ECS Fargate Windows.

- Fixing deadlock when stopping the agent righ when a metadata provider is scheduled.

- Fix a bug where container_include/exclude_metrics was applied on Autodiscovery when using Docker, preventing logs collection configured through container_include/exclude_logs.

- Fix inclusion of ``registry.json`` file in flare

- Fixes an issue where the agent would remove tags from pods or containers
  around 5 minutes after startup of either the agent itself, or the pods or
  containers themselves.

- APM: SQL query obfuscation doesn't drop redacted literals from the obfuscated query when they are preceded by a SQL comment.

- The Kube State Metrics Core check supports VerticalPodAutoscaler metrics.

- The experimental OTLP endpoint now uses the StartTimestamp field for reset detection on cumulative metrics transformations.

- Allow configuring process discovery check in the process agent when both regular process and container checks are off.

- Fix disk check reporting /dev/root instead of the actual
  block device path and missing its tags when tag_by_label
  is enabled.

- Remove occasionally hanging autodiscovery errors 
  from the agent status once a pod is deleted.


.. _Release Notes_7.33.0_Other Notes:

Other Notes
-----------

- The Windows installer only creates the datadog.yaml file on new installs.


.. _Release Notes_7.32.4:

7.32.4 / 6.32.4
======

.. _Release Notes_7.32.4_Prelude:

Prelude
-------

Release on: 2021-12-22


- JMXFetch: Remove all dependencies on ``log4j`` and use ``java.util.logging`` instead.

.. _Release Notes_7.32.3:

7.32.3 / 6.32.3
======

.. _Release Notes_7.32.3_Prelude:

Prelude
-------

Release on: 2021-12-15

.. _Release Notes_7.32.3_Security Notes:

- Upgrade the log4j dependency to 2.12.2 in JMXFetch to fully address `CVE-2021-44228 <https://nvd.nist.gov/vuln/detail/CVE-2021-44228>`_ and `CVE-2021-45046 <https://nvd.nist.gov/vuln/detail/CVE-2021-45046>`_

.. _Release Notes_7.32.2:

7.32.2 / 6.32.2
======

.. _Release Notes_7.32.2_Prelude:

Prelude
-------

Release on: 2021-12-11


.. _Release Notes_7.32.2_Security Notes:

Security Notes
--------------

- Set ``-Dlog4j2.formatMsgNoLookups=True`` when starting the JMXfetch process to mitigate vulnerability described in `CVE-2021-44228 <https://nvd.nist.gov/vuln/detail/CVE-2021-44228>`_


.. _Release Notes_7.32.1:

7.32.1 / 6.32.1
======

.. _Release Notes_7.32.1_Prelude:

Prelude
-------

Release on: 2021-11-18


.. _Release Notes_7.32.1_Bug Fixes:

Bug Fixes
---------

- On ECS, fix the volume of calls to `ListTagsForResource` which led to ECS API throttling.

- Fix incorrect use of a namespaced PID with the host procfs when parsing mountinfo to ensure debugfs is mounted correctly.
  This issue was preventing system-probe startup in AWS ECS. This issue could also surface in other containerized environments
  where PID namespaces are in use and ``/host/proc`` is mounted.

- Fixes system-probe startup failure due to kernel version parsing on Linux 4.14.252+.
  This specifically was affecting versions of Amazon Linux 2, but could affect any Linux kernel in the 4.14 tree with sublevel >= 252.


.. _Release Notes_7.32.0:

7.32.0 / 6.32.0
======

.. _Release Notes_7.32.0_Prelude:

Prelude
-------

Release on: 2021-11-09

- Please refer to the `7.32.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7320>`_ for the list of changes on the Core Checks


.. _Release Notes_7.32.0_Upgrade Notes:

Upgrade Notes
-------------

- APM: Change default profiling intake to use v2 endpoint.

- CSPM the check subcommand is now part of the security-agent compliance.


.. _Release Notes_7.32.0_New Features:

New Features
------------

- On Kubernetes, add a `kube_priority_class` tag on metrics coming from pods with a priority class.

- Priority class name of pods are now collected and sent to the orchestration endpoint

- Autodiscovery can now resolve template variables and environment variables in log configurations.

- The Windows installer now offers US5 as a new site choice.

- APM: New telemetry was added to measure `/v.*/traces` endpoints latency and response size.
  These metrics are `datadog.trace_agent.receiver.{rate_response_bytes,serve_traces_ms}`.

- APM: Metrics are now available for Windows Pipes and UDS connections via datadog.trace_agent.receiver.{uds_connections,pipe_connections}.

- Introduce a new configuration parameter ``container_env_as_tags``
  to allow converting containerd containers' environment variables into tags.

- The "containerd" check is now supported on Windows.

- Add experimental support for writing agent-side CSPM compliance checks in Rego.

- Runtime security can now attach span/trace to event.

- Provides alternative implementation for process collection on Windows using performance counters.

- Add multi-line auto-sensing when tailing logs from file.
  It checks the 1000 first lines (or waits 30 seconds, whichever is first)
  when tailing for a list of known timestamp formats. If the 
  number of matched lines is greater than the threshold it 
  switches to the MultiLineHandler with the pattern matching
  the timestamp format. The pattern chosen is saved in the log
  config and is reused if the file rotates.  Use the new global config 
  parameter ``logs_config.auto_multi_line_detection`` to enable
  the feature for the whole agent, or the per log integration config parameter ``auto_multi_line_detection``
  to enable the feature on a case by case basis.

- Added *experimental* support for OTLP metrics via
  experimental.otlp.{http_port,grpc_port} or their corresponding
  environment variables (DD_OTLP_{HTTP,GRPC}_PORT).

- Created a new process discovery check. This is a lightweight check that runs every 4 hours by default, and collects
  process metadata, so that Datadog can suggest potential integrations for the user to enable.

- Added new executable `readsecret_multiple_providers.sh` that allows the
  agent to read secrets both from files and Kubernetes secrets. Please refer
  to the `docs <https://docs.datadoghq.com/agent/guide/secrets-management>`_
  for more details.


.. _Release Notes_7.32.0_Enhancement Notes:

Enhancement Notes
-----------------

- KSM core check has a new `labels_as_tags` parameter to configure which pod labels should be used as datadog tag in an easier way than with the `label_joins` parameter.

- Add `namespace` to snmp listener config

- Remove `network_devices` from `datadog.yaml` configuration

- kubernetes state core check: add `kubernetes_state.job.completion.succeeded` and `kubernetes_state.job.completion.failed` metrics to report job completion as metrics in addition to the already existing service check.

- Add `use_device_id_as_hostname` in snmp check and snmp_listener configuration to use DeviceId as hostname for metrics and service checks

- APM: The maximum allowed tag value length has been increased to 25,000 bytes.

- Reduce memory usage when checks report new metrics every run. Most metrics are removed
  after two check runs without new samples. Rate, historate and monotonic count will be
  kept in memory for additional 25 hours after that. Number of check runs and the
  additional time can be changed with `check_sampler_bucket_commits_count_expiry` and
  `check_sampler_stateful_metric_expiration_time`. Metric expiration can be disabled
  entirely by setting `check_sampler_expire_metrics` to `false`.

- CSPM reports the agent version as part of the events

- Agents are now built with Go1.16.  This will have one user-visible change:
  on Linux, the process-level RSS metric for agent processes will be
  reduced from earlier versions.  This reflects a change in how memory
  usage is calculated, not a reduction in used memory, and is an artifact
  of the Go runtime `switching from MADV_FREE to MADV_DONTNEED
  <https://golang.org/doc/go1.16#runtime>`_.

- Tag Kubernetes containers with ``image_id`` tag.

- Eliminates the need to synchronize state between regular and RT process collection.

- APM: Added a configuration option to set the API key separately for Live
  Debugger. It can be set via `apm_config.debugger_api_key` or
  `DD_APM_DEBUGGER_API_KEY`.

- Update EP forwarder config to use intake v2 for ndm metadata

- Remove the `reason` tag from the `kubernetes_state.job.failed` metric to reduce cardinality

- the runtime security module of system-probe is now powered by DataDog/ebpf-manager instead of DataDog/ebpf.

- Security Agent: use exponential backoff for log warning when the security agent fails to
  connect to the system probe.

- APM: OTLP traces now supports semantic conventions from version 1.5.0 of the OpenTelemetry specification.

- Show enabled autodiscovery sources in the agent status

- Add namespace to SNMP integration and SNMP Listener to disambiguate
  devices with same IP.

- Add snmp corecheck autodiscovery

- Enable SNMP device metadata collection by default

- Reduced CPU usage when origin detection is used.

- The Windows installer now prioritizes user name from the command line over stored registry entries


.. _Release Notes_7.32.0_Bug Fixes:

Bug Fixes
---------

- Make sure ``DD_ENABLE_METADATA_COLLECTION="false"`` prevent all host metadata emission, including the initial one.

- Most checks are stripping tags with an empty value. KSM was missing this logic so that KSM specific metrics could have a tag with an empty value.
  They will now be stripped like for any other check.

- Fixed a regression that was preventing the Agent from retrying kubelet and docker connections in case of failure.

- Fix the cgroup collector to correctly pickup Cloud Foundry containers.

- Fix an issue where the orchestrator check would stop sending
  updates when run on as a cluster-check.

- Port python-tuf CVE fix on the embedded Python 2
  see `<https://github.com/theupdateframework/python-tuf/security/advisories/GHSA-wjw6-2cqr-j4qr>`_.

- Fix some string logging in the Windows installer.

- The flare command now correctly copies agent logs located in subdirectories
  of the agent's root log directory.

- Kubernetes state core check: `job.status.succeeded` and `job.status.failed` gauges were not sent when equal 0. 0 values are now sent.

- Tag Namespace and PV and PVC metrics correctly with ``phase`` instead of ``pod_phase``
  in the Kube State Metrics Core check.


.. _Release Notes_7.31.1:

7.31.1
======

.. _Release Notes_7.31.1_Prelude:

Prelude
-------

Release on: 2021-09-28

.. _Release Notes_7.31.1_Bug Fixes:

Bug Fixes
---------

- Fix CSPM not sending intake protocol causing lack of host tags.

.. _Release Notes_7.31.0:

7.31.0 / 6.31.0
======

.. _Release Notes_7.31.0_Prelude:

Prelude
-------

Release on: 2021-09-13

- Please refer to the `7.31.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7310>`_ for the list of changes on the Core Checks


.. _Release Notes_7.31.0_New Features:

New Features
------------

- Added `hostname_file` as a configuration option that can be used to set
  the Agent's hostname.

- APM: add a new HTTP proxy endpoint /appsec/proxy forwarding requests to Datadog's AppSec Intake API.

- Add a new parameter (auto_exit) to allow the Agent to exit automatically based on some condition. Currently, the only supported method "noprocess", triggers an exit if no other processes are visible to the Agent (taking into account HOST_PROC). Only available on POSIX systems.

- Allow specifying the destination for dogstatsd capture files, this
  should help drop captures on mounted volumes, etc. If no destination
  is specified the capture will default to the current behavior.

- Allow capturing/replaying dogstatsd traffic compressed with zstd.
  This feature is now enabled by default for captures, but can still
  be disabled.

- APM: Added endpoint for proxying Live Debugger requests.

- Adds the ability to change `log_level` in the process agent at runtime using ``process-agent config set log_level <log-level>``

- Runtime-security new command line allowing to trigger runtime security agent self test.


.. _Release Notes_7.31.0_Enhancement Notes:

Enhancement Notes
-----------------

- Introduce a `container_exclude_stopped_age` configuration option to allow
  the Agent to not autodiscover containers that have been stopped for a
  certain number of hours (by default 22). This makes restarts of the Agent
  not re-send logs for these containers.

- Add two new parameters to allow customizing APIServer connection parameters (CAPath, TLSVerify) without requiring to use a fully custom kubeconfig.

- Leverage Cloud Foundry application metadata to automatically tag Cloud Foundry containers. A label or annotation prefixed with ``tags.datadoghq.com/`` is automatically picked up and used to tag the application container when the cluster agent is configured to query the CC API.

- The ``agent configcheck`` command prints a message for checks that matched a
  container exclusion rule.

- Add calls to Cloudfoundry API for space and organization data to tag application containers with more up-to-date information compared to BBS API.

- The ``agent diagnose`` and ``agent flare`` commands no longer create error-level log messages when the diagnostics fail.
  These message are logged at the "info" level, instead.

- With the dogstatsd-replay feature allow specifying the number of
  iterations to loop over the capture file. Defaults to 1. A value
  of 0 loops forever.

- Collect net stats metrics (RX/TX) for ECS Fargate in Live Containers.

- EKS Fargate containers are tagged with ``eks_fargate_node``.

- The `agent flare` command will now include an error message in the
  resulting "local" flare if it cannot contact a running agent.

- The Kube State Metrics Core check sends a new metric ``kubernetes_state.pod.count``
  tagged with owner tags (e.g ``kube_deployment``, ``kube_replica_set``, ``kube_cronjob``, ``kube_job``).

- The Kube State Metrics Core check tags ``kubernetes_state.replicaset.count`` with a ``kube_deployment`` tag.

- The Kube State Metrics Core check tags ``kubernetes_state.job.count`` with a ``kube_cronjob`` tag.

- The Kube State Metrics Core check adds owner tags to pod metrics.
  (e.g ``kube_deployment``, ``kube_replica_set``, ``kube_cronjob``, ``kube_job``)

- Improve accuracy and reduce false positives on the collector-queue health
  check

- Support posix-compliant flags for process-agent. Shorthand flags for "p" (pid), "i" (info), and "v" (version) are
  now supported.

- The Agent now embeds Python-3.8.11, an upgrade from
  Python-3.8.10.

- APM: Updated the obfuscator to replace digits in IDs of SQL statement in addition to table names,
  when this option is enabled.

- The logs-agent now retries on an HTTP 429 response, where this had been treated as a hard failure.
  The v2 Event Intake will return 429 responses when it is overwhelmed.

- Runtime security now exposes change_time and modification_time in SECL.

- Add security-agent config file to flare

- Add ``min_collection_interval`` config to ``snmp_listener``

- TCP log collectors have historically closed sockets that are idle for more
  than 60 seconds.  This is no longer the case.  The agent relies on TCP
  keepalives to detect failed connections, and will otherwise wait indefinitely
  for logs to arrive on a TCP connection.

- Enhances the secrets feature to support arbitrarily named user
  accounts running the datadog-agent service. Previously the
  feature was hardcoded to `ddagentuser` or Administrator accounts
  only.


.. _Release Notes_7.31.0_Deprecation Notes:

Deprecation Notes
-----------------

- Deprecated non-posix compliant flags for process agent. A warning should now be displayed if one is detected.


.. _Release Notes_7.31.0_Bug Fixes:

Bug Fixes
---------

- Add `send_monotonic_with_gauge`, `ignore_metrics_by_labels`, 
  and `ignore_tags` params to prometheus scrape. Allow values 
  defaulting to `true` to be set to `false`, if configured.

- APM: Fix bug in SQL normalization that resulted in negative integer values to be normalized with an extra minus sign token.

- Fix an issue with autodiscovery on CloudFoundry where in case an application instance crash, a new integration configuration would not be created for the new app instance.

- Auto-discovered checks will not target init containers anymore in Kubernetes.

- Fixes a memory leak when the Agent is running in Docker environments. This
  leak resulted in memory usage growing linearly, corresponding with the
  amount of containers ever ran while the current Agent process was also
  running. Long-lived Agent processes on nodes with a lot of container churn
  would cause the Agent to eventually run out of memory.

- Fixes an issue where the `docker.containers.stopped` metric would have
  unpredictable tags. Now all stopped containers will always be reported with
  the correct tags.

- Fixes bug in enrich tags logic while a dogstatsd capture replay is in
  process; previously when a live traffic originID was not found in the
  captured state, no tags were enriched and the live traffic tagger was
  wrongfully skipped.

- Fixes a packaging issue on Linux where the unixodbc configuration files in
  /opt/datadog-agent/embedded/etc would be erased during Agent upgrades.

- Fix hostname detection when Agent is running on-host and monitoring containerized workload by not using hostname coming from containerized providers (Docker, Kubernetes)

- Fix default mapping for statefulset label in Kubernetes State Metric Core check.

- Fix handling of CPU metrics collected from cgroups when cgroup files are missing.

- Fix a bug where the status command of the security agent
  could crash if the agent is not fully initialized.

- Fixed a bug where the CPU check would not work within a container on Windows.

- Flare generation is no longer subject to the `server_timeout` configuration,
  as gathering all of the information for a flare can take quite some time.

- [corechecks/snmp] Support inline profile definition

- Fixes a bug where the Agent would hold on to tags from stopped ECS EC2 (but
  not Fargate) tags forever, resulting in increased memory consumption on EC2
  instances handling a lot of short scheduled tasks.

- On non-English Windows, the Agent correctly parses the output of `netsh`.


.. _Release Notes_7.31.0_Other Notes:

Other Notes
-----------

- The datadog-agent, datadog-iot-agent and datadog-dogstatsd deb packages now have a weak dependency (`Recommends:`) on the datadog-signing-keys package.


.. _Release Notes_7.30.2:

7.30.2
======

.. _Release Notes_7.30.2_Prelude:

Prelude
-------

Release on: 2021-08-23

This is a Windows-only release.

.. _Release Notes_7.30.2_Bug Fixes:

Bug Fixes
---------

- On Windows, disables ephemeral port range detection.  Fixes crash on non
  EN-US windows

.. _Release Notes_7.30.1:

7.30.1
======

.. _Release Notes_7.30.1_Prelude:

Prelude
-------

Release on: 2021-08-20

- Please refer to the `7.30.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7301>`_ for the list of changes on the Core Checks


.. _Release Notes_7.30.0:

7.30.0 / 6.30.0
======

.. _Release Notes_7.30.0_Prelude:

Prelude
-------

Release on: 2021-08-12

- Please refer to the `7.30.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7300>`_ for the list of changes on the Core Checks


.. _Release Notes_7.30.0_New Features:

New Features
------------

- APM: It is now possible to enable internal profiling of the trace-agent. Warning however that this will incur additional billing charges and should not be used unless agreed with support.

- APM: Added *experimental* support for Opentelemetry collecting via
  experimental.otlp.{http_port,grpc_port} or their corresponding
  environment variables (DD_OTLP_{HTTP,GRPC}_PORT).

- Kubernetes Autodiscovery now supports additional template variables:
  ``%%kube_pod_name%%``, ``%%kube_namespace%%`` and ``%%kube_pod_uid%%``.

- Add support for SELinux related events, like boolean value updates or enforcment status changes.


.. _Release Notes_7.30.0_Enhancement Notes:

Enhancement Notes
-----------------

- Reveals useful information within a SQL execution plan for Postgres.

- Add support to provide options to the obfuscator to change the behavior.

- APM: Added additional tags to profiles in AWS Fargate environments.

- APM: Main hostname acquisition now happens via gRPC to the Datadog Agent.

- Make the check_sampler bucket expiry configurable based on the number of `CheckSampler` commits.

- The cri check no longer sends metrics for stopped containers, in line with
  containerd and docker checks. These metrics were all zeros in the first
  place, so no impact is expected.

- Kubernetes State Core check: Job metrics corresponding to a Cron Job are tagged with a ``kube_cronjob`` tag.

- Environment autodiscovery is now used to selectively activate providers (kubernetes, docker, etc.) inside each component (tagger, host tags, hostname).

- When using a `secret_backend_command` STDERR is always logged with a debug log level. This eases troubleshooting a
  user's `secret_backend_command` in a containerized environment.

- `secret_backend_timeout` has been increased from 5s to 30s. This increases support for the slow to load
  Python script used for `secret_backend_command`. This was an issue when importing large libraries in a
  containerized environment.

- Increase default timeout to sync Kubernetes Informers from 2 to 5 seconds.

- The Kube State Metrics Core checks adds the global user-defined tags (``DD_TAGS``) by the default.

- If the new ``log_all_goroutines_when_unhealthy`` configuration parameter is set to true,
  when a component is unhealthy, log the stacktraces of the goroutines to ease the investigation.

- The amount of time the agent waits before scanning for new logs is now configurable with `logs_config.file_scan_period`

- Flares now include goroutine blocking and mutex profiles if enabled. New flare options
  were added to collect new profiles at the same time as cpu profile.

- Add a section about container inclusion/exclusion errors
  to the agent status command.

- Runtime Security now provide kernel related information
  as part of the flare.

- Python interpreter ``sys.executable`` is now set to the appropriate interpreter's
  executable path. This should allow ``multiprocessing`` to be able to spawn new
  processes since it will try to invoke the Python interpreter instead of the Agent
  itself. It should be noted though that the Pyton packages injected at runtime by
  the Agent are only available from the main process, not from any sub-processes.

- Add a single entrypoint script in the agent docker image.
  This script will be leveraged by a new version of the Helm chart.

- [corechecks/snmp] Add bulk_max_repetitions config

- Add device status snmp corecheck metadata 

- [snmp/corecheck] Add interface.id_tags needed to correlated metadata interfaces with interface metrics

- In addition to the existing ``/readsecret.py`` script, the Agent container image
  contains another secret helper script ``/readsecret.sh``, faster and more reliable.

- Consider pinned CPUs (cpusets) when calculating CPU limit from cgroups.


.. _Release Notes_7.30.0_Bug Fixes:

Bug Fixes
---------

- APM: Fix SQL obfuscation on postgres queries using the tilde operator.

- APM: Fixed an issue with the Web UI on Internet Explorer.

- APM: The priority sampler service catalog is no longer unbounded. It is now limited to 5000 service & env combinations.

- Apply the `max_returned_metrics` parameter from prometheus annotations, 
  if configured.

- Removes noisy error logs when collecting Cloud Foundry application containers

- For dogstatsd captures, Only serialize to disk the portion of buffers
  actually used by the payloads ingested, not the full buffer.

- Fix a bug in cgroup parser preventing from getting proper metrics in Container Live View when using CRI-O and systemd cgroup manager.

- Avoid sending duplicated ``datadog.agent.up`` service checks.

- When tailing logs from docker with `DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE=true` and a 
  source container label is set the agent will now respect that label and use it as the source. 
  This aligns the behavior with tailing from the docker socket. 

- On Windows, when the host shuts down, handles the ``PreShutdown`` message to avoid the error ``The DataDog Agent service terminated unexpectedly.  It has done this 1 time(s).  The following corrective action will be taken in 60000 milliseconds: Restart the service.`` in Event Viewer.

- Fix label joins in the Kube State Metrics Core check.

- Append the cluster name, if found, to the hostname for 
  ``kubernetes_state_core`` metrics.

- Ensure the health probes used as Kubernetes liveness probe are not failing in case of issues on the network or on an external component.

- Remove unplanned call between the process-agent and the the DCA when the
  orchestratorExplorer feature is disabled.

- [corechecks/snmp] Set default oid_batch_size to 5. High oid batch size can lead to timeouts.

- Agent collecting Docker containers on hosts with a lot of container churn
  now uses less memory by properly purging the respective tags after the
  containers exit. Other container runtimes were not affected by the issue.


.. _Release Notes_7.30.0_Other Notes:

Other Notes
-----------

- APM: The trace-agent no longer warns on the first outgoing request retry,
  only starting from the 4th.

- All Agent binaries are now compiled with Go ``1.15.13``

- JMXFetch upgraded to `0.44.2` https://github.com/DataDog/jmxfetch/releases/0.44.2

- Build environment changes:
  
  * omnibus-software: [cacerts] updating with latest: 2021-07-05 (#399)
  * omnibus-ruby: Support 'Recommends' dependencies for deb packages (#122)

- Runtime Security doesn't set the service tag with the
  `runtime-security-agent` value by default.


.. _Release Notes_7.29.1:

7.29.1
======

.. _Release Notes_7.29.1_Prelude:

Prelude
-------

Release on: 2021-07-13

This is a linux + docker-only release.


.. _Release Notes_7.29.1_New Features:

New Features
------------

- APM: Fargate stats and traces are now correctly computed, aggregated and present the expected tags.


.. _Release Notes_7.29.1_Bug Fixes:

Bug Fixes
---------

- APM: The value of the default env is now normalized during trace-agent initialization.


.. _Release Notes_7.29.0:

7.29.0 / 6.29.0
======

.. _Release Notes_7.29.0_Prelude:

Prelude
-------

Release on: 2021-06-24

- Please refer to the `7.29.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7290>`_ for the list of changes on the Core Checks


.. _Release Notes_7.29.0_Upgrade Notes:

Upgrade Notes
-------------

- Upgrade Docker base image to ubuntu:21.04 as new stable release.


.. _Release Notes_7.29.0_New Features:

New Features
------------

- New `extra_tags` setting and `DD_EXTRA_TAGS` environment variable can be
  used to specify additional host tags.

- Add network devices metadata collection

- APM: The obfuscator adds two new features (`dollar_quoted_func` and `keep_sql_alias`). They are off by default. For more details see PR 8071.
  We do not recommend using these features unless you have a good reason or have been recommended by support for your specific use-case.

- APM: Add obfuscator support for Postgres dollar-quoted string constants.

- Tagger state will now be stored for dogstatsd UDS traffic captures
  with origin detection. The feature will track the incoming traffic,
  building a map of traffic source processes and their source containers,
  then storing the relevant tagger state into the capture file. This will
  allow to not only replay the traffic, but also load a snapshot of the
  tagger state to properly tag replayed payloads in the dogstatsd pipeline.

- New `host_aliases` setting can be used to add custom host aliases in
  addition to aliases obtained from cloud providers automatically.

- Paths can now be relsolved using an eRPC request.

- Add time comparison support in SECL allow to write rules
  such as: `open.file.path == "/etc/secret" && process.created_at > 5s`


.. _Release Notes_7.29.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add the following new metrics to the ``kubernetes_state_core``.
  * ``node.ephemeral_storage_allocatable```
  * ``node.ephemeral_storage_capacity``

- Agent can now set hostname based on Azure instance metadata. See the new
  ``azure_hostname_style`` configuration option.

- Compliance agents can now generated multiple reports per run.

- Docker and Kubernetes log launchers will now be retried until
  one succeeds instead of falling back to the docker launcher by default. 

- Increase payload size limit for `dbm-metrics` from `1 MB` to `20 MB`.

- Expose new `batch_max_size` and `batch_max_content_size` config settings for all logs endpoints.

- Adds improved cadence/resolution captures/replay to dogstatsd traffic
  captures. The new file format will store payloads with nanosecond
  resolution. The replay feature remains backward-compatible.

- Support fetching host tags using ECS task and EKS IAM roles.

- Improve the resiliency of the ``datadog-agent check`` command when running Autodiscovered checks.

- Adding the hostname to the host aliases when running on GCE

- Display more information when the error ``Could not initialize instance`` happens.
  JMXFetch upgraded to `0.44.0 <https://github.com/DataDog/jmxfetch/releases/0.44.0>`_

- Kubernetes pod with short-lived containers won't have a few logs of lines
  duplicated with both container tag (the stopped one and the running one) anymore
  while logs are being collected.
  Mount ``/var/log/containers`` and use ``logs_config.validate_pod_container_id``
  to enable this feature.

- The kube state metrics core check now tags pod metrics with a ``reason`` tag.
  It can be ``NodeLost``, ``Evicted`` or ``UnexpectedAdmissionError``.

- Implement the following synthetic metrics in the ``kubernetes_state_core``.
  * ``cronjob.count``
  * ``endpoint.count``
  * ``hpa.count``
  * ``vpa.count`

- Add system.cpu.interrupt on linux.

- Authenticate logs http input requests using the API key header rather than the URL path.

- Upgrade embedded Python 3 from 3.8.8 to 3.8.10. See
  `Python 3.8's changelog <https://docs.python.org/release/3.8.10/whatsnew/changelog.html>`_.

- Show autodiscovery errors from pod annotations in agent status.

- Paths are no longer limited to segments of 128 characters and a depth of 16. Each segment can now be up to 255 characters (kernel limit) and with a depth of up to 1740 parents.

- Add loader as ``snmp_listener.loader`` config

- Make SNMP Listener configs compatible with SNMP Integration configs

- The `agent stream-logs` command will use less CPU while idle.


.. _Release Notes_7.29.0_Security Notes:

Security Notes
--------------

- Redact the whole annotation "kubectl.kubernetes.io/last-applied-configuration" to ensure we don't expose secrets.


.. _Release Notes_7.29.0_Bug Fixes:

Bug Fixes
---------

- Imports the value of `non_local_traffic` to `dogstatsd_non_local_traffic`
  (in addition to `apm_config.non_local_traffic`) when upgrading from
  Datadog Agent v5.

- Fixes the Agent using 100% CPU on MacOS Big Sur.

- Declare `database_monitoring.{samples,metrics}` as known keys in order to remove "unknown key" warnings on startup.

- Fixes the container_name tag not being updated after Docker containers were
  renamed.

- Fixes CPU utilization being underreported on Windows hosts with more than one physical CPU.

- Fix CPU limit used for Live Containers page in ECS Fargate environments.

- Fix bug introduced in 7.26 where default checks were schedueld on ECS Fargate due to changes in entrypoint scripts.

- Fix a bug that can make the agent enable incompatible Autodiscovery listeners.

- An error log was printed when the creation date or the started date 
  of a fargate container was not found in the fargate API payload. 
  This would happen even though it was expected to not have these dates
  because of the container being in a given state. 
  This is now fixed and the error is only printed when it should be.

- Fix the default value of the configuration option ``forwarder_storage_path`` when ``run_path`` is set.
  The default value is ``RUN_PATH/transactions_to_retry`` where RUN_PATH is defined by the configuration option ``run_path``.

- In some cases, compliance checks using YAML file with JQ expressions were failing due to discrepencies between YAML parsing and gojq handling.

- On Windows, fixes inefficient string conversion

- Reduce CPU usage when logs agent is unable to reach an http endpoint.

- Fixed no_proxy depreciation warning from being logged too frequently.
  Added better warnings for when the proxy behavior could change. 

- Ignore CollectorStatus response from orchestrator-intake in the process-agent to prevent changing realtime mode interval to default 2s.

- Fixes an issue where the Agent would not retry resource tags collection for
  containers on ECS if it could retrieve only a subset of tags. Now it will
  keep on retrying until the complete set of tags is collected.

- Fix noisy configuration error when specifying a proxy config and using secrets management.

- Reduce amount of log messages on windows when tailing log files.


.. _Release Notes_7.29.0_Other Notes:

Other Notes
-----------

- JMXFetch upgraded to `0.44.1 <https://github.com/DataDog/jmxfetch/releases/0.44.1>`_


.. _Release Notes_7.28.1:

7.28.1
======

.. _Release Notes_7.28.1_Prelude:

Prelude
-------

Release on: 2021-05-31

- Please refer to the `7.28.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7281>`_ for the list of changes on the Core Checks


.. _Release Notes_7.28.0:

7.28.0 / 6.28.0
======

.. _Release Notes_7.28.0_Prelude:

Prelude
-------

Release on: 2021-05-26

- Please refer to the `7.28.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7280>`_ for the list of changes on the Core Checks


.. _Release Notes_7.28.0_Upgrade Notes:

Upgrade Notes
-------------

- Change base Docker image used to build the Agent images, moving from ``debian:bullseye`` to ``ubuntu:20.10``.
  In the future the Agent will follow Ubuntu stable versions.

- Windows Docker images based on Windows Core are now provided. Checks that didn't work on Nano should work on Core.


.. _Release Notes_7.28.0_New Features:

New Features
------------

- APM: Add a new feature flag ``component2name`` which determines the ``component`` tag value
  on a span to become its operation name. This facititates compatibility with Opentracing.

- Adds a functionality to allow capturing and replaying
  of UDS dogstatsd traffic.

- Expose new ``aggregator.submit_event_platform_event`` python API with two supported event types:
  ``dbm-samples`` and ``dbm-metrics``.

- Runtime security reports environment variables.

- Runtime security now reports command line arguments as part of the
  exec events.

- The ``args_flags`` and ``args_options`` were added to the SECL
  language to ease the writing of runtime security rules based
  on command line arguments.
  ``args_flags`` is used to catch arguments that start by either one
  or two hyphen characters but do not accept any associated value.

  Examples:

  - ``version`` is part of ``args_flags`` for the command ``cat --version``
  - ``l`` and ``n`` both are in ``args_flags`` for the command ``netstat -ln``
  - ``T=8`` and ``width=8`` both are in ``args_options`` for the command
    ``ls -T 8 --width=8``.

- Add support for ARM64 to the runtime security agent


.. _Release Notes_7.28.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add ``oid_batch_size`` configuration as init and instance config

- Add ``oid_batch_size`` config to snmp_listener

- Group the output of ``agent tagger-list`` by entity and by source.

- On Windows on a Domain Controller, if no domain name is specified, the installer will use the controller's joined domain.

- Windows installer can now use the command line key ``EC2_USE_WINDOWS_PREFIX_DETECTION`` to set the config
  value of ``ec2_use_windows_prefix_detection``

- APM: The trace writer will now consider 408 errors to be retriable.

- Build RPMs that can be installed in FIPS mode. This change doesn't affect SUSE RPMs.

  RPMs are now built with RPM 4.15.1 and have SHA256 digest headers, which are required by RPM on CentOS 8/RHEL 8 when running in FIPS mode.

  Note that newly built RPMs are no longer installable on CentOS 5/RHEL 5.

- Make the check_sampler bucket expiry configurable

- The Agent can be configured to replace colon ``:`` characters in the ECS resource tag keys by underscores ``_``.
  This can be done by enabling ``ecs_resource_tags_replace_colon: true`` in the Agent config file
  or by configuring the environment variable ``DD_ECS_RESOURCE_TAGS_REPLACE_COLON=true``.

- Add ``jvm.gc.old_gen_size`` as an alias for ``Tenured Gen``.
  Prevent double signing of release artifacts.

- JMXFetch upgraded to `v0.44.0 <https://github.com/DataDog/jmxfetch/releases/0.44.0>`_.

- The ``kubernetes_state_core`` check now collects two new metrics ``kubernetes_state.pod.age`` and ``kubernetes_state.pod.uptime``.

- Improve ``logs/sender`` throughput by adding optional concurrency for serializing & sending payloads.

- Make kube_replica_set tag low cardinality

- Runtime Security now supports regexp in SECL rules.

- Add loader tag to snmp telemetry metrics

- Network Performance Monitoring for windows now collects DNS stats, connections will be shows in the networks -> DNS page.


.. _Release Notes_7.28.0_Deprecation Notes:

Deprecation Notes
-----------------

- For internal profiling of agent processes, the ``profiling`` option
  has been renamed to ``internal_profiling`` to avoid confusion.

- The single dash variants of the system-probe flags are now deprecated. Please use ``--config`` and ``--pid`` instead.


.. _Release Notes_7.28.0_Bug Fixes:

Bug Fixes
---------

- APM: Fixes bug where long service names and operation names were not normalized correctly.

- On Windows, fixes a bug in process agent in which the process agent
  would become unresponsive.

- The Windows installer compares the DNS domain name and the joined domain name using a case-insensitive compare.
  This avoids an incorrect warning when the domain names match but otherwise have different cases.

- Replace usage of ``runtime.NumCPU`` when used to compute metrics related to CPU Hosts. On some Unix systems,
  ``runtime.NumCPU`` can be influenced by CPU affinity set on the Agent, which should not affect the metrics
  computed for other processes/containers. Affects the CPU Limits metrics (docker/containerd) as well as the
  live containers page metrics.

- Fix issue where Kube Apiserver cache sync timeout configuration is not used.

- Fix the usage of ``DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL`` and ``DD_ORCHESTRATOR_EXPLORER_MAX_PER_MESSAGE`` environment variables.

- Fix a ``panic`` that could occur in Docker AD listener when doing ``docker inspect`` fails

- Fix a small leak where the Agent in some cases keeps in memory identifiers corresponding to dead objects (pods, containers).

- Log file byte count now works correctly on Windows.

- Agent log folder on Mac is moved from ``/var/log/datadog`` to ``/opt/datadog-agent/logs``. A link will be created at
  ``/var/log/datadog`` pointing to ``/opt/datadog-agent/logs`` to maintain the compatibility. This is to workaround the
  issue that some Mac OS releases purge ``/var/log`` folder on ugprade.

- Packaging: ensure only one pip3 version is shipped in ``embedded/`` directory

- Fix eBPF runtime compilation errors with ``tcp_queue_length`` and ``oom_kill`` checks on Ubuntu 20.10.

- Add a validation step before accepting metrics set in HPAs.
  This ensures that no obviously-broken metric is accepted and goes on to
  break the whole metrics gathering process.

- The Windows installer now log only once when it fails to replace a property.

- Windows installer will not abort if the Server service is not running (introduced in 6.24.0/7.24.0).


.. _Release Notes_7.28.0_Other Notes:

Other Notes
-----------

- The Agent, Logs Agent and the system-probe are now compiled with Go ``1.15.11``

- Bump embedded Python 3 to ``3.8.8``


.. _Release Notes_7.27.1:

7.27.1 / 6.27.1
======

.. _Release Notes_7.27.1_Prelude:

Prelude
-------

Release on: 2021-05-07

This is a Windows-only release (MSI and Chocolatey installers only).

.. _Release Notes_7.27.1_Bug Fixes:

Bug Fixes
---------

- On Windows, exit system-probe if process-agent has not queried for connection data for 20 consecutive minutes.
  This ensures excessive system resources are not used while connection data is not being sent to Datadog.


.. _Release Notes_7.27.0:

7.27.0 / 6.27.0
======

.. _Release Notes_7.27.0_Prelude:

Prelude
-------

Release on: 2021-04-14

- Please refer to the `7.27.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7270>`_ for the list of changes on the Core Checks


.. _Release Notes_7.27.0_Upgrade Notes:

Upgrade Notes
-------------

- SECL and JSON format were updated to introduce the new attributes. Legacy support was added to avoid breaking
  existing rules.

- The `overlay_numlower` integer attribute that was reported for files
  and executables was unreliable. It was replaced by a simple boolean
  attribute named `in_upper_layer` that is set to true when a file
  is either only on the upper layer of an overlayfs filesystem, or
  is an altered version of a file present in a base layer.


.. _Release Notes_7.27.0_New Features:

New Features
------------

- APM: Add support for AIX/ppc64. Only POWER8 and above is supported.

- Adds support for Kubernetes namespace labels as tags extraction (kubernetes_namespace_labels_as_tags).

- Add snmp corecheck implementation in go

- APM: Tracing clients no longer need to be sending traces marked
  with sampling priority 0 (AUTO_DROP) in order for stats to be correct.

- APM: A new discovery endpoint has been added at the /info path. It reveals
  information about a running agent, such as available endpoints, version and
  configuration.

- APM: Add support for filtering tags by means of apm_config.filter_tags or environment
  variables DD_APM_FILTER_TAGS_REQUIRE and DD_APM_FILTER_TAGS_REJECT.

- Dogstatsd clients can now choose the cardinality of tags added by origin detection per metrics
  via the tag 'dd.internal.card' ("low", "orch", "high").

- Added two new metrics to the Disk check: read_time and write_time.

- The Agent can store traffic on disk when the in-memory retry queue of the 
  forwarder limit is reached. Enable this capability by setting 
  `forwarder_storage_max_size_in_bytes` to a positive value indicating 
  the maximum amount of storage space, in bytes, that the Agent can use 
  to store traffic on disk.

- PCF Containers custom tags can be extracted from environment
  variables based on an include and exclude lists mechanism.

- NPM is now supported on Windows, for Windows versions 2016 and above.

- Runtime security now report command line arguments as part of the
  exec events.

- Process credentials are now tracked by the runtime security agent. Various user and group attributes are now
  collected, along with kernel capabilities.

- File metadata attributes are now available for all events. Those new attributes include uid, user, gid, group, mode,
  modification time and change time.

- Add config parameters to enable fim and runtime rules.

- Network Performance Monitoring for Windows instruments DNS.  Network data from Windows hosts will be tagged with the domain tag, and the DNS page will show data for Windows hosts.


.. _Release Notes_7.27.0_Enhancement Notes:

Enhancement Notes
-----------------

- Improves sensitive data scrubbing in URLs

- Includes UTC time (unless already in UTC+0) and millisecond timestamp in status logs. Flare archive filename now timestamped in UTC.

- Automatically set debug log_level when the '--flare' option is used with the  JMX command

- Number of matched lines is displayed on the status page for each source using multi_line log processing rules.

- Add public IPv4 for EC2/GCE instances to host network metadata.

- Add ``loader`` config to snmp_listener

- Add snmp corecheck extract value using regex

- Remove agent MaxNumWorkers hard limit that cap the number of check runners
  to 25. The removal is motivated by the need for some users to run thousands
  of integrations like snmp corecheck.

- APM: Change in the stats payload format leading to reduced CPU and memory usage.
  Use of DDSketch instead of GKSketch to aggregate distributions leading to more accurate high percentiles.

- APM: Removal of sublayer metric computation improves performance of the trace agent (CPU and memory).

- APM: All API endpoints now respond with the "Datadog-Agent-Version" HTTP response header.

- Query application list from Cloud Foundry Cloud Controller API to get up-to-date application names for tagging containers and metrics.

- Introduce a clc_runner_id config option to allow overriding the default
  Cluster Checks Runner identifier. Defaults to the node name to make it
  backwards compatible. It is intended to allow binpacking more than a single
  runner per node.

- Improve migration path when shifting docker container tailing
  from the socket to file. If tailing from file for Docker
  containers is enabled, container with an existing entry
  relative to a socket tailer will continue being tailed
  from the Docker socket unless the following newly introduced
  option is set to true:  ``logs_config.docker_container_force_use_file``
  It aims to allow smooth transition to file tailing for Docker
  containers.

- (Unix only) Add `go_core_dump` flag to generate core dumps on Agent crashes

- JSON payload serialization and compression now uses shared input and output buffers to reduce
  total allocations in the lifetime of the agent.

- On Windows the comments in the datadog.yaml file are preserved after installation.

- Add kube_region and kube_zone tags to node metrics reported by the kube-state-metrics core check

- Implement the following synthetic metrics in the ``kubernetes_state_core`` check to mimic the legacy ``kubernetes_state`` one.
  * ``persistentvolumes.by_phase``
  * ``service.count``
  * ``namespace.count``
  * ``replicaset.count``
  * ``job.count``
  * ``deployment.count``
  * ``daemonset.count``
  * ``statefulset.coumt``

- Minor improvements to agent log-stream command. Fixed timestamp, added host name, 
  use redacted log message instead of raw message. 

- NPM - Improve accuracy of retransmits tracking on kernels >=4.7

- Orchestrator explorer collection is no longer handled by the cluster-agent directly but
  by a dedicated check.

- prometheus_scrape.checks may now be defined as an environmnet variable DD_PROMETHEUS_SCRAPE_CHECKS formatted as JSON

- Runtime security module doesn't stop on first policies file
  load error and now send an event with a report of the load.

- Sketch series payloads are now compressed as a stream to reduce 
  buffer allocations.

- The Datadog Agent won't try to connect to kubelet anymore if it's not running in a Kubernetes cluster.


.. _Release Notes_7.27.0_Known Issues:

Known Issues
------------

- On Linux kernel versions < 3.15, conntrack (used for NAT info for connections)
  sampling is not supported, and conntrack updates will be aborted if a higher
  rate of conntrack updates from the system than set by
  system_probe_config.conntrack_rate_limit is detected. This is done to limit
  excessive resource consumption by the netlink conntrack update system. To
  keep using this system even with a high rate of conntrack updates, increase
  the system_probe_config.conntrack_rate_limit. This can potentially lead to
  higher cpu usage.


.. _Release Notes_7.27.0_Deprecation Notes:

Deprecation Notes
-----------------

- APM: Sublayer metrics (trace.<SPAN_NAME>.duration and derivatives) computation
  is removed from the agent in favor of new sublayer metrics generated in the backend.


.. _Release Notes_7.27.0_Bug Fixes:

Bug Fixes
---------

- Fixes bug introduced in #7229

- Adds a limit to the number of DNS stats objects the DNSStatkeeper can have at any given time. This can alleviate memory issues on hosts doing high numbers of DNS requests where network performance monitoring is enabled. 

- Add tags to ``snmp_listener`` network configs. This is needed since user
  switching from Python SNMP Autodiscovery will expect to have tags to be
  available with Agent SNMP Autodiscovery (snmp_listener) too.

- APM: When UDP is not available for Dogstatsd, the trace-agent can now use any other
  available alternative, such as UDS or Windows Pipes.

- APM: Fixes a bug where nested SQL queries may occasionally result in bad obfuscator output.

- APM: All Datadog API key usage is sanitized to exclude newlines and other control characters.

- Exceeding the conntrack rate limit (system_probe_config.conntrack_rate_limit)
  would result in conntrack updates from the system not being processed
  anymore

- Address issue with referencing the wrong repo tag for Docker image by
  simplifying logic in DockerUtil.ResolveImageNameFromContainer to prefer
  Config.Image when possible.

- Fix kernel version parsing when subversion/patch is > 255, so eBPF program loading does not fail.

- Agent host tags are now correctly removed from the in-app host when the configured ``tags``/``DD_TAGS`` list is empty or not defined.

- Fixes scheduling of non-working container checks introduced by environment autodiscovery in 7.26. Features can now be exluded from autodiscovery results through `autoconfig_exclude_features`.
  Example: autoconfig_exclude_features: ["docker","cri"] or DD_AUTOCONFIG_EXCLUDE_FEATURES="docker cri"
  Fix typo in variable used to disable environment autodiscovery and make it usable in `datadog.yaml`. You should now set `autoconfig_from_environment: false` or `DD_AUTOCONFIG_FROM_ENVIRONMENT=false`

- Fixes limitation of runtime autodiscovery which would not allow to run containerd check without cri check enabled. Fixes error logs in non-Kubernetes environments.

- Fix missing tags on Dogstatsd metrics when DD_DOGSTATSD_TAG_CARDINALITY=orchestrator (for instance, task_arn on Fargate)

- Fix a panic in the `system-probe` part of the `tcp_queue_length` check when running on nodes with several CPUs.

- Fix agent crashes from Python interpreter being freed too early. This was
  most likely to occur as an edge case during a shutdown of the agent where
  the interpreter was destroyed before the finalizers for a check were
  invoked by finalizers.

- Do not make the liveness probe fail in case of network connectivity issue.
  However, if the agent looses network connectivity, the readiness probe may still fail.

- On Windows, using process agent, fixes the virtual CPU count when the 
  device has more than one physical CPU (package)).

- On Windows, fixes problem in process agent wherein windows processes
  could not completely exit.

- (macOS only) Apple M1 chip architecture information is now correctly reported.

- Make ebpf compiler buildable on non-GLIBC environment.

- Fix a bug preventing pod updates to be sent due to the Kubelet exposing
  unreliable resource versions.

- Silence INFO and WARNING gRPC logs by default. They can be re-enabled by
  setting GRPC_GO_LOG_VERBOSITY_LEVEL to either INFO or WARNING.


.. _Release Notes_7.27.0_Other Notes:

Other Notes
-----------

- Network monitor now fails to load if conntrack initialization fails on
  system-probe startup. Set network_config.ignore_conntrack_init_failure
  to true to reverse this behavior.

- When generating the permissions.log file for a flare, if the owner of a file
  no longer exists in the system, return its id instead instead of failing.

- Upgrade embedded openssl to ``1.1.1k``.


.. _Release Notes_7.26.0:

7.26.0 / 6.26.0
======

.. _Release Notes_7.26.0_Prelude:

Prelude
-------

Release on: 2021-03-02

- Please refer to the `7.26.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7260>`_ for the list of changes on the Core Checks


.. _Release Notes_7.26.0_Upgrade Notes:

Upgrade Notes
-------------

- ``forwarder_retry_queue_payloads_max_size`` takes precedence over the deprecated
  ``forwarder_retry_queue_max_size``. If ``forwarder_retry_queue_max_size`` 
  is not set, you are not affected by this change. If 
  ``forwarder_retry_queue_max_size`` is set, but 
  ``forwarder_retry_queue_payloads_max_size`` is not set, the Agent uses
  ``forwarder_retry_queue_max_size * 2MB`` 
  as the value of ``forwarder_retry_queue_payloads_max_size``. It is 
  recommended to configure ``forwarder_retry_queue_payloads_max_size`` and 
  remove ``forwarder_retry_queue_max_size`` from the Agent configuration.

- Docker image: remove Docker volumes for ``/etc/datadog-agent`` and ``/tmp`` 
  as it prevents to inherit from Datadog Agent image. It was originally done 
  to allow read-only rootfs on Kubernetes, so in order to continue supporting 
  this feature, relevant volumes are created in newer Kubernetes manifest or 
  Helm chart >= 2.6.9

.. _Release Notes_7.26.0_New Features:

New Features
------------

- APM: Support SQL obfuscator feature to replace consecutive digits in table names.

- APM: Add an endpoint to receive apm stats from tracers.

- Agent discovers by itself which container AD features and checks should be
  scheduled without having to specify any configuration. This works for
  Docker, Containerd, ECS/EKS Fargate and Kubernetes.
  It also allows to support heterogeneous nodes with a single configuration
  (for instance a Kubernetes DaemonSet could cover nodes running Containerd
  and/or Docker - activating relevant configuration depending on node
  configuration).
  This feature is activated by default and can be de-activated by setting
  environment variable ``AUTCONFIG_FROM_ENVIRONMENT=false``.

- Adds a new agent command ``stream-logs`` to stream the logs being processed by the agent.
  This will help diagnose issues with log integrations.

- Submit host tags with log events for a configurable time duration
  to avoid potential race conditions where some tags might not be
  available to all backend services on freshly provisioned instances.

- Added no_proxy_nonexact_match as a configuration setting which 
  allows non-exact URL and IP address matching. The new behavior uses 
  the go http proxy function documented here 
  https://godoc.org/golang.org/x/net/http/httpproxy#Config 
  If the new behavior is disabled, a warning will be logged if a url or IP 
  proxy behavior will change in the future. 

- The Quality of Service of pods is now collected and sent to the orchestration endpoint.

- Runtime-security new command line allowing to trigger a process cache dump..

- Support Prometheus Autodiscovery for Kubernetes Pods.

- The core agent now exposes a gRPC API to expose tags to the other agents.
  The following settings are now introduced to allow each of the agents to use
  this API (they all default to false):
  
  - apm_config.remote_tagger
  - logs_config.remote_tagger
  - process_config.remote_tagger

- New perf map usage metrics.

- Add unofficial arm64 support to network tracer in system-probe.

- system-probe: Add optional runtime compilation of eBPF programs.


.. _Release Notes_7.26.0_Enhancement Notes:

Enhancement Notes
-----------------

- APM: Sublayer metrics (trace.<SPAN_NAME>.duration and derivatives) computation
  in agent can be disabled with feature flags disable_sublayer_spans, disable_sublayer_stats.
  Reach out to support with questions about this metric.

- APM: Automatically activate non-local trafic (i.e. listening on 0.0.0.0) for APM in containerized environment if no explicit setting is set (bind_host or apm_non_local_traffic)

- APM: Add a tag allowing trace metrics from synthetic data to 
  be aggregated independently.

- Consider the task level resource limits if the container level resource limits aren't defined on ECS Fargate.

- Use the default agent transport for host metadata calls.
  This allows usage of the config ``no_proxy`` setting for host metadata calls.
  By default cloud provider IPs are added to the transport's ``no_proxy`` list.
  Added config flag ``use_proxy_for_cloud_metadata`` to disable this behavior. 

- GOMAXPROCS is now set automatically to match the allocated CPU cgroup quota.
  GOMAXPROCS can now also be manually specified and overridden in millicore units.
  If no quota or GOMAXPROCS value is set it will default to the original behavior.

- Added ``--flare`` flag to ``jmx (list|collect)`` commands to save check results to the agent logs directory.
  This enables flare to pick up jmx command results.

- Kubernetes events are now tagged with kube_service, kube_daemon_set, kube_job and kube_cronjob.
  Note: Other object kinds are already supported (pod_name, kube_deployment, kube_replica_set).

- Expose logs agent pipeline latency in the status page.

- Individual DEB packages are now signed.

- Docker container, when not running in a Kubernetes
  environment may now be tailed from their log file.
  The Agent must have read access to /var/lib/docker/containers
  and Docker containers must use the JSON logging driver.
  This new option can be activated using the new configuration
  flag ``logs_config.docker_container_use_file``.

- File tailing from a kubernetes pod annotation is
  now supported. Note that the file path is relative
  to the Agent and not the pod/container bearing
  the annotation.


.. _Release Notes_7.26.0_Bug Fixes:

Bug Fixes
---------

- APM: Group arrays of consecutive '?' identifiers

- Fix agent panic when UDP port is busy and dogstatsd_so_rcvbuf is configured.

- Fix a bug that prevents from reading the correct container resource limits on ECS Fargate.

- Fix parsing of dogstatsd event strings that contained negative lengths for
  event title and/or event text length.

- Fix sending duplicated kubernetes events.

- Do not invoke the secret backend command (if configured) when the agent
  health command/agent container liveness probe is called.

- Fix parsing of CLI options of the ``agent health`` command


.. _Release Notes_7.26.0_Other Notes:

Other Notes
-----------

- Bump gstatus version from 1.0.4 to 1.0.5.

- JMXFetch upgraded from `0.41.0 <https://github.com/DataDog/jmxfetch/releases/0.41.0>`_
  to `0.42.0 <https://github.com/DataDog/jmxfetch/releases/0.42.0>`_


.. _Release Notes_7.25.1:

7.25.1
======

.. _Release Notes_7.25.1_Prelude:

Prelude
-------

Release on: 2021-01-26


.. _Release Notes_7.25.1_Bug Fixes:

Bug Fixes
---------

- Fix "fatal error: concurrent map read and map write" due to reads of
  a concurrently mutated map in inventories.payload.MarshalJSON()

- Fix an issue on arm64 where non-gauge metrics from Python checks
  were treated as gauges.

- On Windows, fixes uninstall/upgrade problem if core agent is not running
  but other services are.

- Fix NPM UDP destination address decoding when source address ends with `.8` during offset guessing.

- On Windows, changes the password generating algorithm to have a minimum
  length of 16 and a maximum length of 20 (from 12-18).  Improves compatibility
  with environments that have longer password requirements.

=============
Release Notes
=============

.. _Release Notes_7.25.0:

7.25.0 / 6.25.0
======

.. _Release Notes_7.25.0_Prelude:

Prelude
-------

Release on: 2021-01-14

- Please refer to the `7.25.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7250>`_ for the list of changes on the Core Checks


.. _Release Notes_7.25.0_New Features:

New Features
------------

- Add `com.datadoghq.ad.tags` container auto-discovery label in AWS Fargate environment.

- Package the gstatus command line tool binary for GlusterFS integration metric collection.

- Queried domain can be tracked as part of DNS stats

- APM: The agent is now able to skip top-level span computation in cases when
  the client has marked them by means of the Datadog-Client-Computed-Top-Level
  header.

- APM: The maximum allowed key length for tags has been increased from 100 to 200.

- APM: Improve Oracle SQL obfuscation support.

- APM: Added support for Windows pipes. To enable it, set the pipe path using
  DD_APM_WINDOWS_PIPE_NAME. For more details check `PR #6615 <https://github.com/DataDog/datadog-agent/pull/6615>`_

- Pause containers are now detected and auto excluded based on the `io.kubernetes` container labels.

- APM: new `datadog_agent.obfuscate_sql_exec_plan` function exposed to python
  checks to enable obfuscation of json-encoded SQL Query Execution Plans.

- APM: new `obfuscate_sql_values` option in `apm_config.obfuscation` enabling optional obfuscation
  of SQL queries contained in JSON data collected from some APM services (ES & Mongo)


.. _Release Notes_7.25.0_Enhancement Notes:

Enhancement Notes
-----------------

- Support the ddog-gov.com site option in the Windows
  GUI installer.

- Adds config setting for ECS metadata endpoint client timeout (ecs_metadata_timeout), value in milliseconds.

- Add `loader` config to allow selecting specific loader
  at runtime. This config is available at `init_config`
  and `instances` level.

- Added additional container information to the status page when collect all container logs is enabled in agent status.

- On Windows, it will no longer be required to supply the ddagentuser name
  on upgrade.  Previously, if a non-default or domain user was used, the
  same user had to be provided on subsequent upgrades.

- Added `--flare` flag to `agent check` to save check results to the agent logs directory.
  This enables flare to pick up check results.

- Added new config option for JMXFetch collect_default_jvm_metrics that enables/disables
  default JVM metric collection. 

- Allow empty message for DogStatsD events (e.g. "_e{10,0}:test title|")

- Expires the cache key for availability of ECS metadata endpoint used to fetch
  EC2 resource tags every 5 minutes.

- Data coming from kubernetes pods now have new kube_ownerref_kind and
  kube_ownerref_name tags for each of the pod's OwnerRef property, indicating
  its Kind and Name, respectively.

- We improved the way Agents get the Kubernetes cluster ID from the Cluster Agent.
  It used to be that the cluster agent would create a configmap which had to be
  mounted as an env variable in the agent daemonset, blocking the process-agent
  from starting if not found. Now the process-agent will start, only the Kubernetes
  Resources collection will be blocked.

- Events sent by the runtime security agent to the backend use
  a new taxonomy.

- Scrub container args as well for orchestrator explorer.

- Support custom autodiscovery identifiers on Kubernetes using the `ad.datadoghq.com/<container_name>.check.id` pod annotation.

- The CPU check now collects system-wide context switches on Linux.

- Add ``--table`` option to ``agent check`` command to output
  results in condensed tabular format instead of JSON.

- APM: improve performance by changing the msgpack serialization implementation.

- APM: improve the performance of the msgpack deserialization for the v0.5 payload format.

- APM: improve performance of trace processing by removing some heap allocations.

- APM: improve sublayer computation performance by reducing the number of heap allocations.

- APM: improved stats computation performance by removing some string concatenations.

- APM: improved trace signature computation by avoiding heap allocations.

- APM: improve stats computation performance.

- Update from alpine:3.10 to alpine:3.12 the base image in Dogstatsd's Dockerfiles. 


.. _Release Notes_7.25.0_Deprecation Notes:

Deprecation Notes
-----------------

- APM: remove the already deprecated apm_config.extra_aggregators config option.


.. _Release Notes_7.25.0_Bug Fixes:

Bug Fixes
---------

- Fix macos `dlopen` failures by ensuring cmake preserves the required runtime search path.

- Fix memory leak on check unscheduling, which could be noticeable for checks
  submitting large amounts of metrics/tags.

- Exclude pause containers using the `cdk/pause.*` image.

- Fixed missing some Agent environment variables in the flare

- Fix a bug that prevented the logs Agent from discovering the correct init containers `source` and `service` on Kubernetes.

- The logs agent now uses the container image name as logs source instead of 
  `kubernetes` when a standard service value was defined for the container.

- Fixes panic on concurrent map access in Kubernetes metadata tag collector.

- Fixed a bug that could potentially cause missing container tags for check metrics.

- Fix a potential panic on ECS when the ECS API is returning empty docker ID

- Fix systemd check id to handle multiple instances. The fix will make
  check id unique for each different instances.

- Fix missing tags on pods that were not seen with a running container yet.

- Fix snmp listener subnet loop to use correct subnet pointer
  when creating snmpJob object.

- Upgrade the embedded pip version to 20.3.3 to get a newer vendored version of urllib3. 


.. _Release Notes_7.25.0_Other Notes:

Other Notes
-----------

- The Agent, Logs Agent and the system-probe are now compiled with Go ``1.14.12``

- Upgrade embedded ``libkrb5`` Kerberos library to v1.18.3. This version drops support for
  the encryption types marked as "weak" in the `docs of the library <https://web.mit.edu/kerberos/krb5-1.17/doc/admin/conf_files/kdc_conf.html#encryption-types>`_


.. _Release Notes_7.24.1:

7.24.1
======

.. _Release Notes_7.24.1_Bug Fixes:

Prelude
-------

Release on: 2020-12-17


Bug Fixes
---------

- Fix a bug when parsing the current version of an integration that prevented
  upgrading from an alpha or beta prerelease version.

- During a domain installation in a child domain, the Windows installer can now use a user from a parent domain.

- The Datadog Agent had a memory leak where some tags would be collected but
  never cleaned up after their entities were removed from a Kubernetes
  cluster due to their IDs not being recognized. This has now been fixed, and
  all tags are garbage collected when their entities are removed.


.. _Release Notes_7.24.1_Other Notes:

Other Notes
-----------

- Updated the shipped CA certs to latest (2020-12-08)

.. _Release Notes_7.24.0:

7.24.0 / 6.24.0
======

.. _Release Notes_7.24.0_Prelude:

Prelude
-------

Release on: 2020-12-03

- Please refer to the `7.24.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7240>`_ for the list of changes on the Core Checks


.. _Release Notes_7.24.0_Upgrade Notes:

Upgrade Notes
-------------

- tcp_queue_length check: the previous metrics reported by this check (``tcp_queue.rqueue.size``, ``tcp_queue.rqueue.min``, ``tcp_queue.rqueue.max``, ``tcp_queue.wqueue.size``, ``tcp_queue.wqueue.min``, ``tcp_queue.wqueue.max``) were generating too much data because there was one time series generated per TCP connection.
  Those metrics have been replaced by ``tcp_queue.read_buffer_max_usage_pct``, ``tcp_queue.write_buffer_max_usage_pct`` which are aggregating all the connections of a container.
  These metrics are reporting the maximum usage in percent (amount of data divided by the queue capacity) of the busiest buffer.
  Additionally, `only_count_nb_context` option from the `tcp_queue_length` check configuration has been removed and will be ignored from now on.


.. _Release Notes_7.24.0_New Features:

New Features
------------

- Added new configuration flag,
  system_probe_config.enable_conntrack_all_namespaces,
  false by default. When set to true, this will allow system
  probe to monitor conntrack entries (for NAT info) in all
  namespaces that are peers of the root namespace.

- Added JMX version and java runtime version to agent status page

- ``kubernetes_pod_annotations_as_tags`` (``DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS``) now support regex wildcards:
  ``'{"*":"<PREFIX>_%%annotation%%"}'`` can be used as value to collect all pod annotations as tags.
  ``kubernetes_node_labels_as_tags`` (``DD_KUBERNETES_NODE_LABELS_AS_TAGS``) now support regex wildcards:
  ``'{"*":"<PREFIX>_%%label%%"}'`` can be used as value to collect all node labels as tags.
  Note: ``kubernetes_pod_labels_as_tags`` (``DD_KUBERNETES_POD_LABELS_AS_TAGS``) supports this already.

- Listening for conntrack updates from all network namespaces
  (system_probe_config.enable_conntrack_all_namespaces flag) is now turned
  on by default


.. _Release Notes_7.24.0_Enhancement Notes:

Enhancement Notes
-----------------

- Expand pause container image filter

- Adds misconfig check for hidepid=2 option on proc mount.

- It's possible to ignore ``auto_conf.yaml`` configuration files using ``ignore_autoconf`` or ``DD_IGNORE_AUTOCONF``.
  Example: DD_IGNORE_AUTOCONF="redisdb kubernetes_state"

- APM: The trace-agent now automatically sets the GOMAXPROCS value in
  Linux containers to match allocated CPU quota, as opposed to the matching
  the entire node's quota.

- APM: Lowered CPU usage when using analytics.

- APM: Move UTF-8 validation from the span normalizer to the trace decoder, which reduces the number of times each distinct string will be validated to once, which is beneficial when the v0.5 trace format is used.

- Add the config `forwarder_retry_queue_payloads_max_size` which defines the 
  maximum size in bytes of all the payloads in the forwarder's retry queue.

- When enabled, JMXFetch now logs to its own log file. Defaults to ``jmxfetch.log``
  in the default agent log directory, and can be configured with ``jmx_log_file``.

- Added UDS support for JMXFetch
  JMXFetch upgraded to `0.40.3 <https://github.com/DataDog/jmxfetch/releases/0.40.3>`_

- dogstatsd_mapper_profiles may now be defined as an environment variable DD_DOGSTATSD_MAPPER_PROFILES formatted as JSON

- Add orchestrator explorer related section into DCA Status

- Added byte count per log source and display it on the status page.

- APM: refactored the SQL obfuscator to be significantly more efficient.


.. _Release Notes_7.24.0_Deprecation Notes:

Deprecation Notes
-----------------

- IO check: device_blacklist_re has been deprecated in favor of device_exclude_re.

- The config options tracemalloc_whitelist and tracemalloc_blacklist have been
  deprecated in favor of tracemalloc_include and tracemalloc_exclude.


.. _Release Notes_7.24.0_Bug Fixes:

Bug Fixes
---------

- APM: Fix a bug where non-float64 numeric values in apm_config.analyzed_spans
  would disable this functionality.

- Disable stack protector on system-probe to make it buildable on the environments which stack protector is enabled by default.
  
  Some linux distributions like Alpine Linux enable stack protector by default which is not available on eBPF.

- Fix a panic in containerd if retrieved ociSpec is nil

- Fix random panic in Kubelet searchPodForContainerID due to concurrent modification of pod.Status.AllContainers

- Add retries to Kubernetes host tags retrievals, minimize the chance of missing/changing host tags every 30mins

- Fix rtloader build on strict posix environment, e.g. musl libc on Alpine Linux.

- Allows system_probe to be enabled without enabling network performance monitoring.
  
  Set ``network_config.enabled=false`` in your ``system-probe.yaml`` when running the system-probe without networks enabled.

- Fixes truncated output for status of compliance checks in Security Agent.

- Under some circumstances, the Agent would delete all tags for a workload if
  they were collected from different sources, such as the kubelet and docker,
  but deleted from only one of them. Now, the agent keeps track of tags per
  collector correctly.


.. _Release Notes_7.24.0_Other Notes:

Other Notes
-----------

- The utilities provided by the `sysstat` package have been removed: the ``iostat``,
  ``mpstat``, ``pidstat``, ``sar``, ``sadf``, ``cifsiostat`` and ``nfsiostat-sysstat``
  binaries have been removed from the packaged Agent. This has no effect on the
  behavior of the Agent and official integrations, but your custom checks may be
  affected if they rely on these embedded binaries.

- Activate security-agent service by default in the Linux packages of the Agent (RPM/DEB). The security-agent won't be started if the file /etc/datadog-agent/security-agent.yaml does not exist.


.. _Release Notes_7.23.1:

7.23.1 / 6.23.1
======

.. _Release Notes_7.23.1_Prelude:

Prelude
-------

Release on: 2020-10-21

.. _Release Notes_7.23.1_Bug Fixes:

Bug Fixes
---------

- The ``ec2_prefer_imdsv2`` parameter was ignored when fetching
  EC2 tags from the metadata endpoint. This fixes a misleading warning log that was logged
  even when ``ec2_prefer_imdsv2`` was left disabled in the Agent configuration.

- Support of secrets in JSON environment variables, added in `7.23.0`, is
  reverted due to a side effect (e.g. a string value of `"-"` would be loaded as a list). This
  feature will be fixed and added again in a future release.

- The Windows installer can now install on domains where the domain name is different from the Netbios name.


.. _Release Notes_7.23.0:

7.23.0 / 6.23.0
======

.. _Release Notes_7.23.0_Prelude:

Prelude
-------

Release on: 2020-10-06

- Please refer to the `7.23.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7230>`_ for the list of changes on the Core Checks


.. _Release Notes_7.23.0_Upgrade Notes:

Upgrade Notes
-------------

- Network monitoring: enable DNS stats collection by default.


.. _Release Notes_7.23.0_New Features:

New Features
------------

- APM: Decoding errors reported by the `datadog.trace-agent.receiver.error`
  and `datadog.trace_agent.normalizer.traces_dropped` contain more detailed
  reason tags in case of EOFs and timeouts.

- Running the agent flare with the -p flag now includes profiles
  for the trace-agent.

- APM: An SQL query obfuscation cache was added under the feature flag
  DD_APM_FEATURES=sql_cache. In most cases where SQL queries are repeated
  or prepared, this can significantly reduce CPU work.

- Secrets handles are not supported inside JSON value set through environment variables.
  For example setting a secret in a list
  `DD_FLARE_STRIPPED_KEYS='["ENC[auth_token_name]"]' datadog-agent run`

- Add basic support for UTF16 (BE and LE) encoding.
  It should be manually enabled in a log configuration using
  ``encoding: utf-16-be`` or ``encoding: utf-16-le`` other
  values are unsupported and ignored by the agent.


.. _Release Notes_7.23.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add new configuration parameter to allow 'GroupExec' permission on the secret-backend command.
  Set to 'true' the new parameter 'secret_backend_command_allow_group_exec_perm' to activate it.

- Add a map from DNS rcode to count of replies received with that rcode

- Enforces a size limit of 64MB to uncompressed sketch payloads (distribution metrics).
  Payloads above this size will be split into smaller payloads before being sent.

- APM: Span normalization speed has been increased by 15%.

- Improve the ``kubelet`` check error reporting in the output of ``agent status`` in the case where the agent cannot properly connect to the kubelet.

- Add `space_id`, `space_name`, `org_id` and `org_name` as tags to both autodiscovered containers as well as checks found through autodiscovery on Cloud Foundry/Tanzu.

- Improves compliance check status view in the security-agent status command.

- Include compliance benchmarks from github.com/DataDog/security-agent-policies in the Agent packages and the Cluster Agent image.

- Windows Docker image is now based on Windows Server Nano instead of Windows Server Core.

- Allow sending the GCP project ID under the ``project_id:`` host tag key, in addition
  to the ``project:`` host tag key, with the ``gce_send_project_id_tag`` config setting.

- Add `kubeconfig` to GCE excluded host tags (used on GKE)

- The cluster name can now be longer than 40 characters, however
  the combined length of the host name and cluster name must not
  exceed 254 characters.

- When requesting EC2 metadata, you can use IMDSv2 by turning
  on a new configuration option (``ec2_prefer_imdsv2``).

- When tailing logs from container in a kubernetes environment
  long lines (>16kB usually) that got split by the container
  runtime (docker & containerd at least) are now reassembled
  pending they do not exceed the upper message length limit
  (256kB).

- Move the cluster-id ConfigMap creation, and Orchestrator
  Explorer controller instantiation behind the orchestrator_explorer
  config flag to avoid it failing and generating error logs.

- Add caching for sending kubernetes resources for live containers

- Agent log format improvement: logs can have kv-pairs as context to make it easier to get all logs for a given context
  Sample: 2020-09-17 12:17:17 UTC | CORE | INFO | (pkg/collector/runner/runner.go:327 in work) | check:io | Done running check

- The CRI check now supports container exclusion based on container name, image and kubernetes namespace.

- Added a network_config config to the system-probe that allows the
  network module to be selectively enabled/disabled. Also added a
  corresponding DD_SYSTEM_PROBE_NETWORK_ENABLED env var.  The network module
  will only be disabled if the network_config exists and has enabled set to
  false, or if the env var is set to false.  To maintain compatibility with
  previous configs, the network module will be enabled in all other cases.

- Log a warning when a log file is rotated but has not finished tailing the file.

- The NTP check now uses the cloud provider's recommended NTP servers by default, if the Agent
  detects that it's running on said cloud provider.


.. _Release Notes_7.23.0_Deprecation Notes:

Deprecation Notes
-----------------

- `process_config.orchestrator_additional_endpoints` and `process_config.orchestrator_dd_url` are deprecated in favor of:
  `orchestrator_explorer.orchestrator_additional_endpoints` and `orchestrator_explorer.orchestrator_dd_url`.


.. _Release Notes_7.23.0_Bug Fixes:

Bug Fixes
---------

- Allow `agent integration install` to work even if the datadog agent
  configuration file doesn't exist.
  This is typically the case when this command is run from a Dockerfile
  in order to build a custom image from the datadog official one.

- Implement variable interpolation in the tagger when inferring the standard tags
  from the ``DD_ENV``, ``DD_SERVICE`` and ``DD_VERSION`` environment variables

- Fix a bug that was causing not picking checks and logs for containers targeted
  by container-image-based autodiscovery. Or picking checks and logs for
  containers that were not targeted by container-image-based autodiscovery.
  This happened when several image names were pointing to the same image digest.

- APM: Allow digits in SQL literal identifiers (e.g. `1sad123jk`)

- Fixes an issue with not always reporting ECS Fargate task_arn tag due to a race condition in the tag collector.

- The SUSE SysVInit service now correctly starts the Agent as the
  dd-agent user instead of root.

- APM: Allow double-colon operator in SQL obfuscator.

- UDP packets can be sent in two ways. In the "connected" way, a `connect` call is
  made first to assign the remote/destination address, and then packets get sent with the `send`
  function or `sendto` function with destination address set to NULL. In the "unconnected" way,
  packets get sent using `sendto` function with a non NULL destination address. This fix addresss
  a bug where network stats were not being generated for UDP packets sent using the "unconnected"
  way.

- Fix the Windows systray not appearing sometimes (bug introduced with 6.20.0).

- The Chocolatey package now uses a fixed URL to the MSI installer.

- Fix logs tagging inconsistency for restarted containers.

- On macOS, in Agent v6, the unversioned python binaries in
  ``/opt/datadog-agent/embedded/bin`` (example: ``python``, ``pip``) now correctly
  point to the Python 2 binaries.

- Fix truncated cgroup name on copy with bpf_probe_read_str in OOM kill and TCP queue length checks.

- Use double-precision floats for metric values submitted from Python checks.

- On Windows, the ddtray executable now has a digital signature

- Updates the logs package to get the short image name from Kubernetes ContainerSpec, rather than ContainerStatus.
  This works around a known issue where the image name in the ContainerStatus may be incorrect.

- On Windows, the Agent now responds to control signals from the OS and shuts down gracefully.
  Coincidentally, a Windows Agent Container will now gracefully stop when receiving the stop command.


.. _Release Notes_7.23.0_Other Notes:

Other Notes
-----------

- All Agents binaries are now compiled with Go  ``1.14.7``

- JMXFetch upgraded from `0.38.2 <https://github.com/DataDog/jmxfetch/releases/0.38.2>`_
  to `0.39.1 <https://github.com/DataDog/jmxfetch/releases/0.39.1>`_

- Move the orchestrator related settings `process_config.orchestrator_additional_endpoints` and
  `process_config.orchestrator_dd_url` into the `orchestrator_explorer` section.


.. _Release Notes_7.22.1:

7.22.1 / 6.22.1
======

.. _Release Notes_7.22.1_Prelude:

Prelude
-------

Release on: 2020-09-17

- Please refer to the `7.22.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7221>`_ for the list of changes on the Core Checks

.. _Release Notes_7.22.1_Bug Fixes:

Bug Fixes
---------

- Define a default logs file (security-agent.log) for the security-agent.

- Fix segfault when listing Garden containers that are in error state.

- Do not activate security-agent service by default in the Linux packages of the Agent (RPM/DEB). The security-agent was already properly starting and exiting if not activated in configuration.


.. _Release Notes_7.22.0:

7.22.0 / 6.22.0
======

.. _Release Notes_7.22.0_Prelude:

Prelude
-------

Release on: 2020-08-25

- Please refer to the `7.22.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7220>`_ for the list of changes on the Core Checks


.. _Release Notes_7.22.0_New Features:

New Features
------------

- Implements agent-side compliance rule evaluation in security agent using expressions.

- Add IO operations monitoring for Docker check (docker.io.read/write_operations)

- Track TCP connection churn on system-probe

- The new Runtime Security Agent collects file integrity monitoring events.
  It is disabled by default and only available for Linux for now.

- Make security-agent part of automatically started agents in RPM/DEB/etc. packages (will do nothing and exit 0 by default)

- Add support for receiving and processing SNMP traps, and forwarding them as logs to Datadog.

- APM: A new trace ingestion endpoint was introduced at /v0.5/traces which supports a more compact payload format, greatly
  improving resource usage. The spec for the new wire format can be viewed `here <https://github.com/DataDog/datadog-agent/blob/7.22.0/pkg/trace/api/version.go#L21-L69>`_.
  Tracers supporting this change will automatically use the new endpoint.

.. _Release Notes_7.22.0_Enhancement Notes:

Enhancement Notes
-----------------

- Adds a gauge for `system.mem.slab_reclaimable`. This is part of slab
  memory that might be reclaimed (i.e. caches). Datadog 7.x adds
  `SReclaimable` memory, if available on the system, to the
  `system.mem.cached` gauge by default. This may lead to inconsistent
  metrics for clients migrating from Datadog 5.x, where
  `system.mem.cached` didn't include `SReclaimable` memory. Adding a gauge
  for `system.mem.slab_reclaimable` allows inverse calculation to remove
  this value from the `system.mem.cached` gauge.

- Expand GCR pause container image filter

- Kubernetes events for pods, replicasets and deployments now have tags that match the metrics metadata.
  Namely, `pod_name`, `kube_deployment`, `kube_replicas_set`.

- Enabled the collection of the kubernetes resource requirements (requests and limits)
  by bumping the agent-payload dep. and collecting the resource requirements.

- Implements resource fallbacks for complex compliance check assertions.

- Add system.cpu.num_cores metric with the number of CPU cores (windows/linux)

- compliance: Add support for Go custom compliance checks and implement two for CIS Kubernetes

- Make DSD Mapper also map metrics that already contain tags.

- If the retrieval of the AWS EC2 instance ID or hostname fails, previously-retrieved
  values are now sent, which should mitigate host aliases flapping issues in-app.

- Increase default timeout on AWS EC2 metadata endpoints, and make it configurable
  with ``ec2_metadata_timeout``

- Add container incl./excl. lists support for ECS Fargate (process-agent)

- Adds support for a heap profile and cpu profile (of configurable length) to be created and
  included in the flare.

- Upgrade embedded Python 3 to 3.8.5. Link to Python 3.8 changelog: https://docs.python.org/3/whatsnew/3.8.html

  Note that the Python 2 version shipped in Agent v6 continues to be version 2.7.18 (unchanged).

- Upgrade pip to v20.1.1. Link to pip 20.1.1 changelog: https://pip.pypa.io/en/stable/news/#id54

- Upgrade pip-tools to v5.3.1. Link to pip-tools 5.3.1 changelog: https://github.com/jazzband/pip-tools/blob/master/CHANGELOG.md

- Introduces support for resolving pathFrom from in File and Audit checks.

- On Windows, always add the user to the required groups during installation.

- APM: A series of changes to internal algorithms were made which reduced CPU usage between 20-40% based on throughput.


.. _Release Notes_7.22.0_Bug Fixes:

Bug Fixes
---------

- Allow integration commands to work for pre-release versions.

- [Windows] Ensure ``PYTHONPATH`` variable is ignored correctly when initializing the Python runtime.

- Enable listening for conntrack info from all namespaces in system probe

- Fix cases where the resolution of secrets in integration configs would not
  be performed for autodiscovered containers.

- Fixes submission of containers blkio metrics that may modify array after being already used by aggregator. Can cause missing tags on containerd.* metrics

- Restore support of JSON-formatted lists for configuration options passed as environment variables.

- Don't allow pressing the disable button on checks twice.

- Fix `container_include_metrics` support for all container checks

- Fix a bug where the Agent disables collecting tags when the
  cluster checks advanced dispatching is enabled in the Daemonset Agent.

- Fixes a bug where the ECS metadata endpoint V2 would get queried even though it was not configured
  with the configuration option `cloud_provider_metadata`.

- Fix a bug when a kubernetes job has exited after some time the tagger does not update it even if it did change its state.

- Fixes the Agent failing to start on sysvinit on systems with dpkg >= 1.19.3

- The agent was collecting docker container logs (metrics)
  even if they are matching `DD_CONTAINER_EXCLUDE_LOGS`
  (resp. `DD_CONTAINER_EXCLUDE_METRICS`)
  if they were started before the agent. This is now fixed.

- Fix a bug where the Agent would not remove tags for pods that no longer
  exist, potentially causing unbounded memory growth.

- Fix pidfile support on security-agent

- Fixed system-probe not working on CentOS/RHEL 8 due to our custom SELinux policy.
  We now install the custom policy only on CentOS/RHEL 7, where the system-probe is known
  not to work with the default. On other platform the default will be used.

- Stop sending payload for Cloud Foundry applications containers that have no `container_name` tag attached to avoid them showing up in the UI with empty name.


.. _Release Notes_7.22.0_Other Notes:

Other Notes
-----------

- APM: datadog.trace_agent.receiver.* metrics are now also tagged by endpoint_version


.. _Release Notes_7.21.1:

7.21.1
======

.. _Release Notes_7.21.1_Prelude:

Prelude
-------

Release on: 2020-07-22

.. _Release Notes_7.21.1_Bug Fixes:

Bug Fixes
---------

- JMXFetch upgraded to `0.38.2 <https://github.com/DataDog/jmxfetch/releases/0.38.2>`_ to fix Java 7 support.
- Fix init of security-agent - exit properly when no feature requiring it is activated and avoid conflicting with core agent port bindings.

.. _Release Notes_7.21.0:

7.21.0 / 6.21.0
======

.. _Release Notes_7.21.0_Prelude:

Prelude
-------

Release on: 2020-07-16

- Please refer to the `7.21.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7210>`_ for the list of changes on the Core Checks


.. _Release Notes_7.21.0_Upgrade Notes:

Upgrade Notes
-------------

- APM: The maximum allowed payload size by the agent was increased
  from 10MB to 50MB. This could result in traffic increases for
  users which were affected by this issue.

- APM: The maximum connection limit over a 30s period was removed.
  This can result in an increase of tracing data for users that were
  affected by this limitation.


.. _Release Notes_7.21.0_New Features:

New Features
------------

- Add support of new DatadogMetric CRD in DCA. Allows to autoscale based on any valid Datadog query.

- Add packages scripts for dogstatsd that have the same features as the agent: create
  symlink for binary, create dd-agent user and group, setup the service and cleanup
  those when uninstalling.

- Adds OOM Kill probe to ebpf package and corresponding corecheck to the agent.

- The Datadog IoT Agent is now available for 32 bit ARM architecture (armv7l/armhf).

- Add Compliance agent in Cluster Agent to monitor Kubernetes objects

- Add `docker.cpu.limit` and `containerd.cpu.limit` metrics, reporting maximum cpu time (hz or ns) available for each container based on their limits. (Only supported on Linux)

- Addition of a gRPC server and a hostname resolution endpoint,
  including a grpc-gateway that exposes said endpoint as a REST
  service.

- Adding a 'log_format_rfc3339' option to use the RFC3339 format for the log
  time.

- Compliance Agent implementing scheduling of compliance checks for Docker and Kubernetes benchmarks.

- Expose agent's sql obfuscation to python checks via new `datadog_agent.obfuscate_sql` method

- Support installing non-core integrations with the ``integration`` command,
  such as those located in the ``integrations-extras`` repository.


.. _Release Notes_7.21.0_Enhancement Notes:

Enhancement Notes
-----------------

- The Agent ``status`` command now includes the flavor
  of the Agent that is running.

- The Agent GUI now includes the flavor
  of the Agent that is running.

- Adds Tagger information to Datadog Agent flare for support investigations.

- Add a static collector in the tagger package for tags that do not change after pod start (such as
  those from an environment variable).

- Add ``autodiscovery_subnet`` to available SNMP template extra configs

- When enabling `collect_ec2_tags` or `collect_gce_tags` option, EC2/GCE tags
  are now cached to avoid missing tags when user exceed his AWS/GCE quotas.

- Chocolatey package can be installed on Domain Controller

- The Agent now collects the Availability Zone a Fargate Task (using platform
  version 1.4 or later) is running in as an "availability_zone" tag.

- Enabled the collection of the init-containers by bumping the agent-payload dep. and collecting the init-containers.

- The Agent now collects recommended "app.kubernetes.io" Kubernetes labels as
  tags by default, and exposes them under a "kube_app" prefix.

- Docker and Containerd checks now support filtering containers by kube_namespace.

- Add support for sampling to distribution metrics

- Flare now includes the permission information for parents of config and log file directories.

- Collect processes namespaced PID.

- You can now enable or disable the dogstatsd-stats troubleshooting feature at
  runtime using the ``config set dogstatsd_stats`` command of the Agent.

- API Keys are now sanitized for `logs_config` and `additional_endpoints`.

- Upgrade gosnmp to support more authentication and privacy protocols
  for v3 connections.

- Use the standard tag 'service' as a log collection attribute for container's logs
  collected from both kubernetes and docker log sources.

- `agent check` returns non zero exit code when trace malloc is enabled (`tracemalloc_debug: true`) when using python 2

- Added the checksum type to the checksum key itself, as it is deprecated to have a separate
  checksum_type key.

- Add ``lowercase_device_tag`` option to the system ``io`` core check on Windows.
  When enabled, sends metrics with a lowercased ``device`` tag, which is consistent with the
  ``system.io.*`` metrics of Agent v5 and the ``system.disk.*`` metrics of all Agent
  versions.


.. _Release Notes_7.21.0_Bug Fixes:

Bug Fixes
---------

- Fix missing values from cluster-agent status command.

- Add missing ``device_name`` tag in iostats_pdh

- Fixes an issue where DD_TAGS were not applied to EKS Fargate pods and containers.

- Add ``freetds`` linux dep needed for SQL Server to run in Docker Agent.

- APM : Fix parsing of non-ASCII numerals in the SQL obfuscator. Previously
  unicode characters for which unicode.IsDigit returns true could cause a
  hang in the SQL obfuscator

- APM: correctly obfuscate AUTH command.

- Dogstatsd standalone: when running on a systemd-based system, do not stop
  Dogstatsd when journald is stopped or restarted.

- Fix missing logs and metrics for docker-labels based autodiscovery configs after container restart.

- Fix bugs introduced in 7.20.0/6.20.0 in the Agent 5 configuration import command:
  the command would not import some Agent config settings, including ``api_key``,
  and would write some Docker & Kubernetes config settings to wrongly-located files.

- Fixes tag extraction from Kubernetes pod labels when using patterns on
  certain non-alphanumeric label names (e.g. app.kubernetes.io/managed-by).

- Fixes the `/ready` health endpoint on the cluster-agent.

  The `/ready` health endpoint was reporting 200 at the cluster-agent startup and was then, permanently reporting 500 even though the cluster-agent was experiencing no problem.
  In the body of the response, we could see that a `healthcheck` component was failing.
  This change fixes this issue.

- This fix aims to cover the case when the agent is running inside GKE with workload identity enabled.
  If workload identity is enabled, access to /instance/name is forbidden, resulting into an empty host alias.

- Fix hostname resolution issue preventing the Process and APM agents from picking
  up a valid hostname on some containerized environments

- Fix a bug which causes certain configuration options to be ignored by the ``process-agent`` in the presence of a ``system-probe.yaml``.

- Process agent and system probe now correctly accept multiple API keys per endpoint.

- The ``device_name`` tag is not used anymore to populate the ``Device`` field of a series. Only the ``device`` tag is considered.

- Fixes problem on Windows where ddagentuser home directory is left behind.

- Revert upgrade of GoSNMP and addition of extra authentication protocols.

- Add support for examining processes inside Docker containers running under
  systemd cgroups. This also reduces agent logging volume as it's able to
  capture those statistics going forward.

- APM: The agent now exits with code 0 when the API key is not specified. This is so to prevent the Windows SCM
  from restarting the process.


.. _Release Notes_7.21.0_Other Notes:

Other Notes
-----------

- All Agents binaries are now compiled with Go ``1.13.11``.

- In Debug mode, DogStatsD log a warning message when a burst of metrics is detected.

- JMXFetch upgraded to `0.38.0 <https://github.com/DataDog/jmxfetch/releases/0.38.0>`_

- JQuery, used in the web-based agent GUI, has been upgraded to 3.5.1


.. _Release Notes_7.20.2:

7.20.2
======

.. _Release Notes_7.20.2_Prelude:

Prelude
-------

Release on: 2020-06-17

- Please refer to the `7.20.2 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7202>`_ for the list of changes on the Core Checks


.. _Release Notes_7.20.1:

7.20.1
======

.. _Release Notes_7.20.1_Prelude:

Prelude
-------

Release on: 2020-06-11

- Please refer to the `7.20.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7201>`_ for the list of changes on the Core Checks


.. _Release Notes_7.20.0:

7.20.0 / 6.20.0
======

.. _Release Notes_7.20.0_Prelude:

Prelude
-------

Release on: 2020-06-11

- Please refer to the `7.20.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7200>`_ for the list of changes on the Core Checks


.. _Release Notes_7.20.0_New Features:

New Features
------------

- Pod and container tags autodiscovered via pod annotations
  now support multiple values for the same key.

- Install script creates ``install_info`` report

- Agent detects ``install_info`` report and sends it through Host metadata

- Adding logic to get standard ``service`` tag from Pod Metadata Labels.

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

- ``container_exclude_metrics`` and ``container_include_metrics`` can be used to filter metrics collection for autodiscovered containers.
  ``container_exclude_logs`` and ``container_include_logs`` can be used to filter logs collection for autodiscovered containers.

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

- On Linux and MacOS, add a new ``device_name`` tag on IOstats and disk checks.

- Windows installer can use the command line key ``HOSTNAME_FQDN_ENABLED`` to set the config value of ``hostname_fqdn``.

- Add missing ``device_name`` tags on docker, containerd and network checks.
  Make series manage ``device_name`` tag if ``device`` is missing.

- Support custom tagging of docker container data via an autodiscovery "tags"
  label key.

- Improved performances in metric aggregation logic.
  Use 64 bits context keys instead of 128 bits in order to benefit from better
  performances using them as map keys (fast path methods) + better performances
  while computing the hash thanks to inlining.

- Count of DNS responses with error codes are tracked for each connection.

- Latency of successful and failed DNS queries are tracked for each connection.Queries that time out are also tracked separately.

- Enrich dogstatsd metrics with task_arn tag if
  DD_DOGSTATSD_TAG_CARDINALITY=orchestrator.

- More pause containers from ``ecr``, ``gcr`` and ``mcr`` are excluded automatically by the Agent.

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

- Introduce ``kube_cluster_name`` and ``ecs_cluster_name`` tags in addition to ``cluster_name``.
  Add the possibility to stop sending the ``cluster_name`` tag using the parameter ``disable_cluster_name_tag_key`` in Agent config.
  The Agent keeps sending ``kube_cluster_name`` and `ecs_cluster_name` tags regardless of `disable_cluster_name_tag_key`.

- Configure additional process and orchestrator endpoints by environment variable.

- The process agent can be canfigured to collect containers
  from multiple sources (e.g kubelet and docker simultaneously).

- Upgrading the embedded Python 2 to the latest, and final, 2.7.18 release.

- Improve performance of system-probe conntracker.

- Throttle netlink socket on workloads with high connection churn.


.. _Release Notes_7.20.0_Deprecation Notes:

Deprecation Notes
-----------------

- ``container_exclude`` replaces ``ac_exclude``.
  ``container_include`` replaces ``ac_include``.
  ``ac_exclude`` and ``ac_include`` will keep being supported but the Agent ignores them
  in favor of ``container_exclude`` and ``container_include`` if they're defined.


.. _Release Notes_7.20.0_Bug Fixes:

Bug Fixes
---------

- APM: Fix a small programming error causing the "superfluous response.WriteHeader call" warning.

- Fixes missing container stats in ECS Fargate 1.4.0.

- Ensure Python checks are always garbage-collected after they're unscheduled
  by AutoDiscovery.

- Fix for autodiscovered checks not being rescheduled after container restart.

- On Windows, fix calculation of the ``system.swap.pct_free`` metric.

- Fix a bug in the file tailer on Windows where the log-agent would keep a
  lock on the file preventing users from renaming the it.


.. _Release Notes_7.20.0_Other Notes:

Other Notes
-----------

- Upgrade embedded ntplib to ``0.3.4``

- JMXFetch upgraded to `0.36.2 <https://github.com/DataDog/jmxfetch/releases/0.36.2>`_

- Rebranded puppy agent as iot-agent.


.. _Release Notes_7.19.2:

7.19.2 / 6.19.2
======

.. _Release Notes_7.19.2_Prelude:

Prelude
-------

Release on: 2020-05-12

- Please refer to the `7.19.2 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7192>`_ for the list of changes on the Core Checks


.. _Release Notes_7.19.1:

7.19.1
======

.. _Release Notes_7.19.1_Prelude:

Prelude
-------

Release on: 2020-05-04

This release is only an Agent v7 release, as Agent v6 is not affected by the undermentioned bug.

.. _Release Notes_7.19.1_Bug Fixes:

Bug Fixes
---------

- Fix panic in the dogstatsd standalone package when running in a containerized environment.


.. _Release Notes_7.19.0:

7.19.0 / 6.19.0
======

.. _Release Notes_7.19.0_Prelude:

Prelude
-------

Release on: 2020-04-30

- Please refer to the `7.19.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7190>`_ for the list of changes on the Core Checks


.. _Release Notes_7.19.0_Upgrade Notes:

Upgrade Notes
-------------

- Default logs-agent to use HTTPS with compression when possible.
  Starting from this version, the default transport is HTTPS with compression instead of TCP.
  The usage of TCP is kept in the following cases:
    * logs_config.use_tcp is set to true
    * logs_config.socks5_proxy_address is set, because socks5 proxies are not yet supported in HTTPS with compression
    * HTTPS connectivity test has failed: at agent startup, an HTTPS test request is sent to determine if HTTPS can be used

  To force the use of TCP or HTTPS with compression, logs_config.use_tcp or logs_config.use_http can be set to true, respectively.


.. _Release Notes_7.19.0_New Features:

New Features
------------

- The Agent is now available on Chocolatey for Windows

- Add ``--full-sketches`` option to agent check command that displays bins information

- Support logs collection from Kubernetes log files with old Kubernetes versions (< v1.10).

- Expose the new JMXFetch rate with metrics method to test JMX based checks.

- The ``ac_include`` and ``ac_exclude`` auto-discovery parameters now support the
  ``kube_namespace:`` prefix to collect or discard logs and metrics for whole namespaces
  in addition to the ``name:`` and ``image:`` prefixes to filter on container name and image name.

- EKS Fargate containers now appear in the live containers view.
  All processes running inside the EKS Fargate Pod appear in the live processes view
  when `shareProcessNamespace` is enabled in the Pod Spec.

- Add the ability to change log_level at runtime. The agent command
  has been extended to support new operation. For example to set
  the log_level to debug the following command should be used:
  ``agent config set log_level debug``, all runtime-configurable
  settings can be listed using ``agent config list-runtime``. The
  log_level may also be fetched using the ``agent config get log_level``
  command. Additional runtime-editable setting can easily be added
  by following this implementation.

- The ``system-probe`` classifies UDP connections as incoming or outgoing.


.. _Release Notes_7.19.0_Enhancement Notes:

Enhancement Notes
-----------------

- Adds a new config option to the systemd core check. It adds the ability to provide a custom
  mapping from a unit substate to the service check status.

- The systemd core check now collects and submits the systemd version as check metadata.

- Add ``host_provider_id`` tag to Kubernetes events; for AWS instances this is unique in
  contrast to the Kubernetes nodename currently provided with the ``host`` tag.

- On Windows, now reports system.io.r_await and system.io.w_await.
  Metrics are reported from the performance monitor "Avg. Disk sec/Read" and
  "Avg. Disk sec/Write" metrics.

- Allow setting ``is_jmx`` at the instance level, thereby enabling integrations
  to utilize JMXFetch and Python/Go.

- The authentication token file is now only created
  when the agent is launched with the ``agent start`` command
  It prevents command such as ``agent status`` to create
  an authentication token file owned by a wrong user.

- Count of successful DNS responses are tracked for each connection.

- Network information is collected when the agent is running in docker (host mode only).

- Make sure we don't recognize ``sha256:...`` as a valid image name and add fallback to
  .Config.Image in case it's impossible to map ``sha256:...`` to a proper image name

- Extract env, version and service tags from Docker containers

- Extract env, version and service tags from ECS Fargate containers

- Extract env, version and service tags from kubelet

- Log configurations of type ``file`` now accept a new parameter that allows
  to specify whether a log shall be tailed from the beginning
  or the end. It aims to allow whole log collection, including
  events that may occur before the agent first starts. The
  parameter is named ``start_position`` and it can be set to
  ``end`` or ``beginning``, the default value is ``end``.

- Resolve Docker image name using config.Image in the case of multiple image RepoTags

- The agent configcheck command output now scrubs sensitive
  data and prevents API keys, password, token, etc. to
  appear in the console.

- Errors that arise while loading checks configuration
  files are now send with metadata along with checks
  loading errors and running errors so they will show
  up on the infrastructure list in the DataDog app.

- Remove cgroup deps from Docker utils, allows to implement several backends for Docker utils (i.e. Windows)


.. _Release Notes_7.19.0_Bug Fixes:

Bug Fixes
---------

- On Windows, for Python3, add additional C-runtime DLLs to fix missing dependencies (notably for jpype).

- Fix 'check' command segfault when running for more than 1 hour (which could
  happen when using the '-b' option to set breakpoint).

- Fix panic due to concurrent map access in Docker AD provider

- Fix the ``flare`` command not being able to be created for the non-core agents (trace,
  network, ...) when running in a separated container, such as in Helm. A new
  option, ``--local``, has been added to the ``flare`` command to force the
  creation of the archive using the local filesystem and not the one where
  the core agent process is in.

- Fix logs status page section showing port '0' being used when using the
  default HTTPS URL. The status page now show 443.

- Fix S6 behavior when the core agent dies.
  When the core agent died in a multi-process agent container managed by S6,
  the container stayed in an unhealthy half dead state.
  S6 configuration has been modified so that it will now exit in case of
  core agent death so that the whole container will exit and will be restarted.

- On Windows, fixes Process agent memory leak when obtaining process arguments.

- When a DNS name with ".local" is specifed in the parameter DDAGENTUSER_NAME, the correctly finds the corresponding domain.

- Fix an issue where ``conf.yaml.example`` can be missing from ``Add a check`` menu in the Web user interface.

- process-agent and system-probe now clean up their PID files when exiting.

- When the HTTPS transport is used to send logs, send the sourcecategory as the ``sourcecategory:`` tag
  instead of ``ddsourcecategory:``, for consistency with other transports.


.. _Release Notes_7.19.0_Other Notes:

Other Notes
-----------

- All Agents binaries are now compiled with Go ``1.13.8``

- JMXFetch upgraded to 0.36.1. See `0.36.1 <https://github.com/DataDog/jmxfetch/releases/0.36.1>`_
  and `0.36.0 <https://github.com/DataDog/jmxfetch/releases/0.36.0>`_

- The ``statsd_metric_namespace`` option now ignores the following metric
  prefixes: ``airflow``, ``confluent``, ``hazelcast``, ``hive``, ``ignite``,
  ``jboss``, ``sidekiq``


.. _Release Notes_7.18.1:

7.18.1
======

.. _Release Notes_7.18.1_Bug Fixes:

Bug Fixes
---------

- Fix conntrack issue where a large batch of deletion events was killing
  the goroutine polling the netlink socket.

- On Debian and Ubuntu-based systems, remove system-probe SELinux policy
  to prevent install failures.

.. _Release Notes_7.18.0:

7.18.0 / 6.18.0
======

.. _Release Notes_7.18.0_Prelude:

Prelude
-------

Release on: 2020-03-13

- Please refer to the `7.18.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7180>`_ for the list of changes on the Core Checks


.. _Release Notes_7.18.0_Upgrade Notes:

Upgrade Notes
-------------

- APM: Traces containing spans without an operation name will automatically
  be assigned the "unnamed_span" name (previously "unnamed-span").

- On MacOS, the Aerospike integration is no longer available since version 3.10
  of the aerospike-client-python library is not yet available on this platform.

- On MacOS, the IBM WAS integration is no longer available since it
  relies on the lxml package which currently doesn't pass Apple's
  notarization tests.

- On Windows, the embedded Python will no longer use the PYTHONPATH
  environment variable, restricting its access to the Python packages
  installed with the Agent. Set windows_use_pythonpath to true to keep
  the previous behavior.


.. _Release Notes_7.18.0_New Features:

New Features
------------

- Adding new "env" top level config option. This will be added to the host
  tags and propagated to the trace agent.

- Add SystemD integration support for SUSE.

- APM: Add support for calculating trace sublayer metrics for measured spans.

- APM: Add support for computing trace statistics on user-selected spans.

- APM: add support for `version` as another tag in trace metrics.

- Add docker.uptime, cri.uptime, and containerd.uptime metrics for all containers

- Add a warning in the logs-agent section of the agent status to incite users to switch over HTTP transport.

- Send a tag for any ``service`` defined in the ``init_config`` or
  ``instances`` section of integration configuration, with the latter
  taking precedence. This applies to metrics, events, and service checks.


.. _Release Notes_7.18.0_Enhancement Notes:

Enhancement Notes
-----------------

- The min_collection_interval check setting has been
  relocated since Agent 6/7 release. The agent import
  command now include in the right section this setting
  when importing configuration from Agent 5.

- Add new config parameter (dogstatsd_entity_id_precedence) to enable DD_ENTITY_ID
  presence check when enriching Dogstatsd metrics with tags.

- Add an option to exclude log files by name when wildcarding
  directories. The option is named ``exclude_paths``, it can be
  added for each custom log collection configuration file.
  The option accepts a list of glob.

- The status output now shows checks' last execution date
  and the last successful one.

- On debian- and rhel-based systems, system-probe is now set up so that
  it can run in SELinux-enabled environments.

- On Windows, a step to set the ``site`` parameter has been added
  to the graphical installer.

- Added support for inspecting DNS traffic received over TCP to gather DNS information.

- Review the retry strategy used by the agent to connect to external services like docker, kubernetes API server, kubelet, etc.
  In case of failure to connect to them, the agent used to retry every 30 seconds 10 times and then, to give up.
  The agent will now retry after 1 second. It will then double the period between two consecutive retries each time, up to 5 minutes.
  So, After 10 minutes, the agent will keep on retrying every 5 minutes instead of completely giving up after 5 minutes.
  This change will avoid to have to restart the agent if it started in an environment that remains degraded for a while (docker being down for several minutes for example.)

- Adds message field to the ComponentStatus check of kube_apiserver_controlplane.up
  service check.

- Add a config option ``ec2_use_windows_prefix_detection`` to use the EC2 instance id for Windows hosts on EC2.

- Templates used for the agent status command are now
  embedded in the binary at compilation time and thus
  original template files are not required anymore at
  runtime.

- Upgrade ``pip-tools`` and ``wheel`` dependencies for Python 2.

- Improve interpolation performance during conversion from Prometheus and
  OpenMetrics histograms to ddsketch

- Allow sources for the Logs Agent to fallback to the ``service``
  defined in the ``init_config`` section of integration configuration
  to match the global tag that will be emitted.

- Stop doing HTML escaping on agent status command output
  in order to properly display all non-alphanumeric
  characters.

- Upgrade embedded Python 3 to 3.8.1. Link to Python 3.8 changelog: https://docs.python.org/3/whatsnew/3.8.html

  Note that the Python 2 version shipped in Agent v6 continues to be version 2.7.17 (unchanged).

- Removing an RPM of the Datadog Agent will no longer throw missing files warning.

- The agent config command output now scrubs sensitive
  data and prevents API keys, password, token, etc. from
  appearing in the console.

- Add support for the EC2 instance metadata service
  (IMDS) v2 that requires to get a token before any
  metadata query. The agent will still issue
  unauthenticated request first (IMDS v1) before
  switching to token-based authentication (IMDS
  v2) if it fails.

- system-probe no longer needs to be enabled/started separately through systemctl


.. _Release Notes_7.18.0_Bug Fixes:

Bug Fixes
---------

- The `submit_histogram_bucket` API now accepts long integers as input values.

- ignore "origin" tags if the 'dd.internal.entity_id' tag is present in dogstatsd metrics.

- On Windows 64 bit, fix calculation of number of CPUS to handle
  machines with more than 64 CPUs.

- Make ``systemd`` core check handle gracefully missing ``SystemState`` attribute.

- Ignore missing docker label com.datadoghq.ad.check_names instead of showing error logs.

- The `jmx` and `check jmx` command will now properly use the loglevel provided
  with the deprecated `--log-level` flag or the `DD_LOG_LEVEL` environment var if any.

- Fix docker logs when the tailer receives a io.EOF error during a file rotation.

- Fix process-agent potentially dividing by zero when no containers are found.

- Fix process-agent not respecting logger configuration during startup.


.. _Release Notes_7.18.0_Other Notes:

Other Notes
-----------

- Errors mentioning the authentication token are now more specific and
  won't be obfuscated anymore.

- Upgrade embedded openssl to ``1.1.1d``, pyopenssl ``19.0.0`` and
  postgresql client lib to ``9.4.25``.

- Add the Go version used to build Dogstatsd in its `version` command.

- Update `s6-overlay` to `v1.22.1.0` in docker images

- JMXFetch upgraded to `0.35.0 <https://github.com/DataDog/jmxfetch/releases/0.35.0>`_

- Following the upgrade to Python 3.8, the Datadog Agent version ``>= 6.18.0``
  running Python 3 and version ``>= 7.18.0`` are now enforcing UTF8 encoding
  while running checks (and while running pdb debugger with `-b` option on the
  `check` cli command). Previous versions of the Agent were already using this
  encoding by default (depending on the environment variables) without enforcing it.


.. _Release Notes_7.17.2:

7.17.2
======

.. _Release Notes_7.17.2_Prelude:

Prelude
-------

Release on: 2020-02-26

This is a Windows-only release.


.. _Release Notes_7.17.2_Bug Fixes:

Bug Fixes
---------

- On Windows, fixes the Agent 7 installation causing the machine
  to reboot if the C runtime was upgraded when in use.

.. _Release Notes_7.17.1:

7.17.1 / 6.17.1
======

.. _Release Notes_7.17.1_Prelude:

Prelude
-------

Release on: 2020-02-20

- Please refer to the `7.17.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7171>`_ for the list of changes on the Core Checks


.. _Release Notes_7.17.1_Bug Fixes:

Bug Fixes
---------

- Fix a panic in the log agent when the auto-discovery reports new containers to monitor
  and the agent fails to connect to the docker daemon.
  The main setup where this happened is on ECS Fargate where the ECS auto-discovery is watching
  for new containers and the docker socket is not available from the datadog agent.

- Fix DNS resolution for NPM when the system-probe is running in a container on a non-host network.

.. _Release Notes_7.17.0:

7.17.0 / 6.17.0
======

.. _Release Notes_7.17.0_Prelude:

Prelude
-------

Release on: 2020-02-04

- Please refer to the `7.17.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7170>`_ for the list of changes on the Core Checks


.. _Release Notes_7.17.0_Upgrade Notes:

Upgrade Notes
-------------

- Change agents base images to Debian bullseye

- Starting with this version, the containerized Agent never chooses the OS hostname as its hostname when it is running in a dedicated UTS namespace.
  This is done in order to avoid picking container IDs or kubernetes POD names as hostnames, since these identifiers do not reflect the identity of the host they run on.

  This change only affects you if your agent is currently using a container ID or a kubernetes POD name as hostname.
  The hostname of the agent can be checked with ``agent hostname``.
  If the output stays stable when the container or POD of the agent is destroyed and recreated, you’re not impacted by this change.
  If the output changes, it means that the agent was unable to talk to EC2/GKE metadata server, it was unable to get the k8s node name from the kubelet, it was unable to get the hostname from the docker daemon and it is running in its dedicated UTS namespace.
  Under those conditions, you should set explicitly define the host name to be used by the agent in its configuration file.


.. _Release Notes_7.17.0_New Features:

New Features
------------

- Add logic to support querying the kubelet through the APIServer to monitor AWS Fargate on Amazon EKS.

- Add mapping feature to dogstatsd to convert parts of dogstatsd/statsd
  metric names to tags using mapping rules in dogstatsd using wildcard and
  regex patterns.

- Resource tag collection on ECS.

- Add container_mode in journald input to set source/service as Docker image short name when we receive container logs


.. _Release Notes_7.17.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add kube_node_role tag in host metadata for the node role based on the ``kubernetes.io/role`` label.

- Add cluster_name tag in host metadata tags. Cluster name used is read from
  config if set by user or autodiscovered from cloud provider or Kubernetes
  node label.

- The Agent check command displays the distribution metrics.
  The Agent status command displays histogram bucket samples.

- The system-probe will augment network connection information with
  DNS names gathered by inspecting local DNS traffic.

- Users can now use references to capture groups
  in mask sequence replacement_placeholder strings

- Do not apply the metric namespace configured under ``statsd_metric_namespace`` to dogstatsd metrics prefixed with ``datadog.tracer``. Tracer metrics are published with this prefix.


.. _Release Notes_7.17.0_Bug Fixes:

Bug Fixes
---------

- APM: The trace-agent now correctly applies ``log_to_console``, ``log_to_syslog``
  and all other syslog settings.

- Make the log agent continuously retry to connect to docker rather than giving up when docker is not running when the agent is started.
  This is to handle the case where the agent is started while the docker daemon is stopped and the docker daemon is started later while the datadog agent is already running.

- Fixes #4650 [v7] Syntax in /readsecret.py for Py3

- Fixes an issue in Docker where mounting empty directories to disable docker check results in an error.

- Fixes the matching of container id in Tagger (due to runtime prefix) by matching on the 'id' part only

- Fix the node roles to host tags feature by handling the other official Kube way to setting node roles (when multiple roles are required)

- Properly check for "true" value of env var DD_LEADER_ELECTION

- It's possible now to reduce the risk of missing kubernetes tags on initial logs by configuring "logs_config.tagger_warmup_duration".
  Configuring "logs_config.tagger_warmup_duration" delays the send of the first logs of a container.
  Default value 0 seconds, the fix is disabled by default.
  Setting "logs_config.tagger_warmup_duration" to 5 (seconds) should be enough to retrieve all the tags.

- Fix eBPF code compilation errors about ``asm goto`` on Ubuntu 19.04 (Disco)

- Fix race condition in singleton initialization

- On Windows, fixes registration of agent as event log source.  Allows
  agent to correctly write to the Windows event log.

- On Windows, when upgrading, installer will fail if the user attempts
  to assign a configuration file directory or binary directory that is
  different from the original.

- Add logic to support docker restart of containers.

- Fix a Network Performance Monitoring issue where TCP connection direction was incorrectly classified as ``outgoing`` in containerized environments.

- Fixed a few edge cases that could lead to events payloads being rejected by Datadog's intake for being too big.


.. _Release Notes_7.17.0_Other Notes:

Other Notes
-----------

- Upgrade embedded dependencies: ``curl`` to ``7.66.0``, ``autoconf`` to ``2.69``,
  ``procps`` to ``3.3.16``

- JMXFetch upgraded to `0.34.0 <https://github.com/DataDog/jmxfetch/releases/0.34.0>`_

- Bump embedded Python 3 to 3.7.6


.. _Release Notes_7.16.1:

7.16.1 / 6.16.1
========

.. _Release Notes_7.16.1_Prelude:

Prelude
-------

Release on: 2020-01-06

- Please refer to the `7.16.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7161>`_ for the list of changes on the Core Checks


.. _Release Notes_7.16.1_Security Issues:

Security Issues
---------------

- UnixODBC software dependency bumped to 2.3.7 to address `CVE-2018-7409
  <https://access.redhat.com/security/cve/cve-2018-7409>`_.


.. _Release Notes_7.16.0:

7.16.0 / 6.16.0
======

.. _Release Notes_7.16.0_Prelude:

Prelude
-------

Release on: 2019-12-18

This release introduces major version 7 of the Datadog Agent, which starts at v7.16.0. The only change from Agent v6 is that
v7 defaults to Python 3 and only includes support for Python 3. Before upgrading to v7, confirm that any
custom checks you have are compatible with Python 3. See this `guide <https://docs.datadoghq.com/agent/guide/python-3/>`_
for more information.

Except for the supported Python versions, v7.16.0 and v6.16.0 have the same features.

Please refer to the `7.16.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-7160>`_ for the list of changes on the Core Checks


.. _Release Notes_7.16.0_New Features:

New Features
------------

- Add support for SysVInit on SUSE 11.

- Add information on endpoints inside the logs-agent section of the agent status.


.. _Release Notes_7.16.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add Python 3 linter results to status page

- Log a warning when the hostname defined in the configuration will not be used as the in-app hostname.

- Add ``ignore_autodiscovery_tags`` parameter config check.

  In some cases, a check should not receive tags coming from the autodiscovery listeners.
  By default ``ignore_autodiscovery_tags`` is set to false which doesn't change the behavior of the checks.
  The first check that will use it is ``kubernetes_state``.

- Adds a new ``flare_stripped_keys`` config setting to clean up additional
  configuration information from flare.

- Adding a new config option ``exclude_gce_tags``, to configure which metadata
  attribute from Google Cloud Engine to exclude from being converted into
  host tags.

- Extends the docker and containerd checks to include an open file descriptors
  metric. This metric reports the number of open file descriptors per container.

- Allow the Agent to schedule different checks from different sources on the same service.


.. _Release Notes_7.16.0_Bug Fixes:

Bug Fixes
---------

- APM: Added a fallback into the SQL obfuscator to handle SQL engines that treat
  backslashes literally.

- The default list of sensitive keywords for process argument scrubbing now uses wildcards before and after.

- On Windows process agent, fix problem wherein if the agent is unable
  to figure out the process user name, the process info/stats were not
  sent at all.  Now sends all relevant stats without the username

- On windows, correctly deletes python 3 precompiled files (.pyc) in
  the event of an installation failure and rollback

- Logs: tailed files discovered through a configuration entry with
  wildcard will properly have the ``dirname`` tag on all log entries.

- Fix small memory leak in ``datadog_agent.set_external_tags`` when an empty
  source_type dict is passed for a given hostname.

- Carry a custom patch for jaydebeapi to support latest jpype.

- Check that cluster-name provided by configuraiton file are compliant with the same rule as on GKE. Logs an error and ignore it otherwise.


.. _Release Notes_7.16.0_Other Notes:

Other Notes
-----------

- JMXFetch upgraded to `0.33.1 <https://github.com/DataDog/jmxfetch/releases/0.33.1>`_

- JQuery, used in the web base agent GUI, has been upgraded to 3.4.1


.. _Release Notes_6.15.1:

6.15.1
======

.. _Release Notes_6.15.1_Prelude:

Prelude
-------

Release on: 2019-11-27
This release was published for Windows on 2019-12-09.

.. _Release Notes_6.15.1_New Features:

New Features
------------

- Collect IP address from containers in awsvpc mode

.. _Release Notes_6.15.1_Bug Fixes:

Bug Fixes
---------

- Reintroduce legacy checks directory to make legacy AgentCheck import path
  (``from checks import AgentCheck``) work again.

- Systemd integration points are re-ordered so that ``dbus`` is used in
  preference to the systemd private API at ``/run/systemd/private``, as per
  the systemd documentation. This prevents unnecessary logging to the system
  journal when datadog-agent is run without root permissions.


.. _Release Notes_6.15.1_Other Notes:

Other Notes
-----------

- Bump embedded Python to 2.7.17.

.. _Release Notes_6.15.0:


6.15.0
======

.. _Release Notes_6.15.0_Prelude:

Prelude
-------

Release on: 2019-11-05

- Please refer to the `6.15.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6150>`_ for the list of changes on the Core Checks


.. _Release Notes_6.15.0_New Features:

New Features
------------

- Add persistent volume claim as tag (``persistentvolumeclaim:<pvc_name>``) to StatefulSets pods.

- APM: On SQL obfuscation errors, a detailed explanation is presented when DEBUG logging
  level is enabled.

- APM: SQL obfuscation now supports queries with UTF-8 characters.

- Augment network data with DNS information.

- Add an option to disable the cluster agent local fallback for tag collection (disabled by default).

- DNS lookup information is now included with network data via system-probe.

- Add support for the `XX:+UseContainerSupport` JVM option through the
  `jmx_use_container_support` configuration option.

- The Cluster Agent can now collect stats from Cluster Level Check runners
  to optimize its dispatching logic and rebalance the scheduled checks.

- Add a new python API to store and retrieve data.
  `datadog_agent.write_persistent_cache(key, value)` persists the data in
  `value` (as a string), whereas `datadog_agent.read_persistent_cache(key)`
  returns it for usage afterwards.

- Adding support for compression when forwarding logs through HTTPS. Enable it
  by following instructions
  `here <https://docs.datadoghq.com/agent/logs/?tab=httpcompressed#send-logs-over-https>`_

.. _Release Notes_6.15.0_Enhancement Notes:

Enhancement Notes
-----------------

- Migrate the api version of the Deployment and DaemonSet kubernetes objects
  to apps/v1 as older bersions are not supported anymore in k8s 1.16.

- Running the command `check jmx` now runs once JMXFetch with
  the `with-metrics` command instead of just displaying an error.

- Add options ``tracemalloc_whitelist`` and ``tracemalloc_blacklist`` for
  allowing the use of tracemalloc only for specific checks.

- APM: a warning is now issued when important HTTP headers are omitted by clients.

- The system-probe will no longer log excessively when its internal copy of the conntrack table
  is full.  Furthermore, the artificial cap of 65536  on  ``system_probe_config.max_tracked_connections``,
  which controlled the maximum number of conntrack entries seen by the system-probe has been lifted.

- Allow filtering of event types,reason and kind at query time.
  Make the event limit configurable.
  Improve the interaction with the ConfigMap to store the Resource Version.

- The agent will now try to flush data to the backend when before exiting
  (from DogStatsD and checks). This avoid having metrics gap when restarting
  the agent. This behavior can be disable through configuration, see
  `aggregator_stop_timeout` and `forwarder_stop_timeout`.

- Expose metrics for the cluster level checks advanced dispatching.

- Implement API that allows Python checks to send metadata using
  the ``inventories`` provider.


.. _Release Notes_6.15.0_Security Issues:

Security Issues
---------------

- The ddagentuser no longer has write access to the process-agent binary on Windows


.. _Release Notes_6.15.0_Bug Fixes:

Bug Fixes
---------

- Avoid the tagger to log a warning when a docker container is not found.

- Use ``pkg_resources`` to collect the version of the integrations
  instead of importing them.

- On Windows, allow the uninstall to succeed even if the removal of
  the `ddagentuser` fails for some reason.

- APM: double-quoted strings following assignments are now correctly obfuscated.

- APM: Fixed a bug where an inactive ratelimiter would skew stats.

- Fix an issue where the node agent would not retry to connect to the cluster agent for tag collection.

- Fix the appearrance of the status bar icon when using dark mode on macOS

- The process-agent and system-probe agents should ignore SIGPIPE signals.

- Fix the behavior of the diagnose command that would not consider default configuration location
  when run independently

- Fix a bug where the agent would crash when using the docker autodiscovery config provider.

- Do not permit sending events at their first timestamp.

- Fix tag support for NTP check.

- Fixes a typo in the windows service related commands for the process agent CLI.
  Was previously referencing `trace-agent`.

- On Windows, properly installs on Read Only Domain Controller.
  Adds rights to domain-created user in local GPOs.

- Behavioral change on the forwarder healthcheck such that full queues
  will not label the forwarder as unhealthy. Networking or endpoint issues
  are not representative of an unhealthy agent or forwarder.

- The agent is now more resilient to incomplete responses from the kubelet

- On Linux, preserve the script `/opt/datadog-agent/embedded/bin/2to3`
  that relies on the python 2 interpreter, alongside the python 3 one.

- Fix a possible race in autodiscovery where checks & log collection
  would be wrongly unscheduled.

- Minor memory leaks identified and fixed in RTLoader.

- On Windows, fixes installation logging to not include certain sensitive
  data (specifically api key and the ddagentuser password)

- Fixed a few edge cases that could lead to service checks payloads being rejected by Datadog's intake for being too big

- Use pylint directly for py3 validation, removing dependency on a7.


.. _Release Notes_6.15.0_Other Notes:

Other Notes
-----------

- JMXFetch upgraded from 0.31.0 to `0.32.1
  <https://github.com/DataDog/jmxfetch/blob/master/CHANGELOG.md#0321--2019-09-27>`_.

- JMXFetch upgraded from 0.32.1 to 0.33.0
  https://github.com/DataDog/jmxfetch/blob/master/CHANGELOG.md#0330--2019-10-10

.. _Release Notes_6.14.3:

6.14.3
======

.. _Release Notes_6.14.3_Prelude:

Prelude
-------

Release on: 2019-12-05

This release is only available for Windows.

.. _Release Notes_6.14.3_Bug Fixes:

Bug Fixes
---------

- On windows, fixes problem where Agent would intermittently fail to install
  on domain-joined machine, when another Agent was already installed on the
  DC.

.. _Release Notes_6.14.2:

6.14.2
======

.. _Release Notes_6.14.2_Prelude:

Prelude
-------

Released on: 2019-10-29

This release is only available for Windows.

.. _Release Notes_6.14.2_Bug Fixes:

Bug Fixes
---------

- On Windows, allows the install to succeed successfully even in the event
  that the user was not cleaned up successfully from a previous install.

- On Windows, do not remove the home folder of the Agent's user (`dd-agent-user`) on uninstall.

.. _Release Notes_6.14.1:

6.14.1
======

.. _Release Notes_6.14.1_Prelude:

Prelude
-------

Release on: 2019-09-26


.. _Release Notes_6.14.1_Bug Fixes:

Bug Fixes
---------

- Disable debug log lines for the 'hostname' command since it's directly called
  by some Agent components. Fixes hostname resolution issues for APM and Live Process.


.. _Release Notes_6.14.0:

6.14.0
======

.. _Release Notes_6.14.0_Prelude:

Prelude
-------

Release on: 2019-09-16

- Please refer to the `6.14.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6140>`_ for the list of changes on the Core Checks


.. _Release Notes_6.14.0_Upgrade Notes:

Upgrade Notes
-------------

- The GPG key used to sign the Agent RPM packages has been rotated.
  See the dedicated `Agent documentation page
  <https://docs.datadoghq.com/agent/faq/rpm-gpg-key-rotation-agent-6/>`_
  to know how to make sure that the new Agent RPM packages can be installed on
  hosts.

- Update to the configuration of the systemd check: ``unit_names`` is now
  required and only matching units will be monitored, ``unit_regexes``
  configuration has been removed.

- Several metrics sent by the systemd check have been renamed. The integration is now stable.

- All integrations that make HTTP(S) requests now properly fall back to proxy settings defined in
  ``datadog.yaml`` if none are specified at the integration level. If this is undesired, you can
  set ``skip_proxy`` to ``true`` in every instance config or in the ``init_config`` fallback.

.. _Release Notes_6.14.0_New Features:

New Features
------------

- APM: add support for container tagging. It can be used with any client tracer that supports it.

- APM: Incoming TCP connections are now measured in the datadog.trace_agent.receiver.tcp_connections
  metrics with a "status" tag having values: "accepted", "rejected", "timedout" and "errored".

- Allows the user to blacklist source and destination connections by passing IPs or CIDRs as well as port numbers.

- Docker label autodiscovery configurations are now polled more often by default.

- The Agent can now expose runner stats via the CLC Runner API Server, a remotely-accessible authenticated API server.
  The Cluster Agent can use these stats to optimize dispatching cluster level checks.
  The CLC Runner API Server is disabled by default, it must be enabled in the Agent configuration, also the cluster agent must be enabled since it's the only client of the server.
  By default, the server listens on 5005 and its host address must be set to the Agent Pod IP using the Kubernetes downward API.

- [preview] Checks can now send histogram buckets to the agent to be sent as distribution metrics.

- In macOS datadog-agent is now able to start/stop process-agent.

- The Agent now includes a Python 3 runtime to run checks.
  By default, the Python 2 runtime is used. See the dedicated `Agent documentation page
  <https://docs.datadoghq.com/agent/guide/python-3/>`_ for details on how to
  configure the Agent to use the Python 3 runtime and how to migrate checks from
  Python 2 to Python 3.

- High-level RTLoader memory usage statistics exposed as expvars
  on the agent.

- Adding tracemalloc_debug configuration setting (Python3 only).
  Enables Tracemalloc memory profiling on Python3. Enabling this
  option will override the number of check runners to 1 to guarantee
  sequential execution of checks.

- For NTP check, add the option ``use_local_defined_servers``.
  When ``use_local_defined_servers`` is true, use the ntp servers defined in the current host otherwise use the hosts defined in the configuration.


.. _Release Notes_6.14.0_Enhancement Notes:

Enhancement Notes
-----------------

- Show configuration source for each check's instance in the "status" and the
  "configcheck" commands.

- Add a new invoke task, ``rtloader.generate-doc`` which generates
  Doxygen documentation for the rtloader directory and warns about
  documentation errors or warnings.

- Allow the check command to display and/or store memory profiling data.

- For Windows, add a message when the user cannot perform the action in the systray.

- APM: The `datadog.trace_agent.normalizer.traces_dropped` metric now has a new
  reason `payload_too_large` which was confusingly merged with `decoding_error`.

- APM: Bind ``apm_config.replace_tags`` parameter to ``DD_APM_REPLACE_TAGS`` environment variable.
  It accepts a JSON formatted string of the form ``[{"name":"tag_name","pattern":"pattern","repl":"repl_str"}]``

- The default collection interval for host metadata has been reduced from 4
  hours to 30 min.

- Collection interval for the default metadata providers ('host',
  'agent_checks' and 'resources') can now be configured using the
  'metadata_providers' configuration entry.

- Agent commands now honor the DD_LOG_LEVEL env variable if set.

- Distributions: Distribution payloads are now compressed before being sent to
  Datadog if the agent is built with either zlib or zstd.

- Configuration files for core checks in cmd/agent/dist/conf.d/
  have been migated to the new configuration file norm.
  https://docs.datadoghq.com/developers/integrations/new_check_howto/#configuration-file

- When a valid command is passed to the agent but the command fails, don't display the help usage message.

- Add ``private_socket`` configuration to the systemd check. Defaults to ``/run/systemd/private``
  (or ``/host/run/systemd/private`` when using Docker Agent).

- Warnings returned by the Python 3 linter for custom checks are
  now logged in the Agent at the 'debug' level.

- Make NTP check less verbose when a host can't be reached.
  Warn only after 10 consecutive errors.

- Added detection of a network ID which will be used to improve destination
  resolution of network connections.

- Windows events will now display a full text message instead of a JSON
  object. When available, the agent will now enrich the events with status,
  human readable task name and opcode.

- On Windows, adds system.mem.pagefile.* stats, previously available
  only in Agent 5.


.. _Release Notes_6.14.0_Deprecation Notes:

Deprecation Notes
-----------------

- The ``--log-level`` argument in ``agent check`` and ``agent jmx`` commands
  has been deprecated in favor of the DD_LOG_LEVEL env variable.


.. _Release Notes_6.14.0_Bug Fixes:

Bug Fixes
---------

- APM: The ``datadog.trace_agent.receiver.payload_refused`` metric now has language tags
  like its peer metrics.

- The ``agent jmx`` command now correctly takes into account the options in the
  `init_config` section of the JMXFetch integration configs

- Escape message when using JSON log format. This, for example, fixes
  multiline JSON payload when logging a Exception from Python.

- Fix a bug, when a check have its init configuration before that all the tagger collector report tags.

- Fix spikes for ``system.io.avg_q_sz`` metrics on Linux when the kernel counter
  was wrapping back to 0.

- Fix system.io.* metrics on Linux that were off by 1 when the kernel counters
  were wrapping back to 0.

- Fixed placeholder value for the marathon entry point to match the new configuration file layout.

- Fix a ``tagger`` goroutine race issue when adding a new entry in the ``tagger.Store`` and requesting an entry in another goroutine.

- Fix files descriptor leak when tailing a logs file with file rotation and
  the tailer is stuck for instance because of lost connectivity with the logs
  intake endpoint.

- The parameter ``jmx`` is not supported with the command ``check``, the ``jmx`` command should be used instead.

- Fixed NTP timeout not being used from the configuration.

- On Windows, correctly configure the config file if the path includes
  a space.

- When uninstalling the agent, remove ddagentuser home folder.

- APM: Fix incorrect ``traces_dropped`` and ``spans_malformed`` metric counts.

- On Windows, "ddagentuser" (the user context under which the Agent runs),
  is now added to the "Event Log Readers" group, granting access to
  Security event logs.


.. _Release Notes_6.14.0_Other Notes:

Other Notes
-----------

- The Windows agent no longer depends on the Windows WMI service.
  If the WMI service stops for any reason, the Windows agent will no
  longer stop with it.  However, any integrations that do use WMI
  (wmi_check and win32_event_log) will not be able to function until
  the WMI service restarts.

- Ignore the containerd startup script and the kubeconfig as part of the host metadata on GKE.

- JMXFetch upgraded to `0.31.0 <https://github.com/DataDog/jmxfetch/releases/0.31.0>`_

- On Windows, during an uninstall, if the user context for the datadog agent
  is a domain user, the user will no longer be deleted even when the user
  was created by the corresponding install.


.. _Release Notes_6.13.0:

6.13.0
======

.. _Release Notes_6.13.0_Prelude:

Prelude
-------

Release on: 2019-07-24

- Please refer to the `6.13.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6130>`_ for the list of changes on the Core Checks


.. _Release Notes_6.13.0_Upgrade Notes:

Upgrade Notes
-------------

- The ``port`` option in the NTP check configuration is now parsed as an integer instead of a string.


.. _Release Notes_6.13.0_New Features:

New Features
------------

- APM: add support for Unix Domain Sockets by means of the `apm_config.receiver_socket` configuration. It is off by default. When set,
  it must point to a valid sock file.

- APM: API emitted metrics now have a lang_vendor tag when the Datadog-Meta-Lang-Vendor
  HTTP header is sent by clients.

- APM: Resource-based rate limiting in the API can now be completely
  disabled by setting `apm_config.max_memory` and/or `apm_config.max_cpu_percent`
  to the value 0.

- Add support for environment variables in checks' config files
  using the format "%%env_XXXX%%".

- Add new systemd integration to monitor systemd itself
  and the units managed by systemd.

- The total number of bytes received by dogstatsd is now reported by the
  `dogstatsd-udp/Bytes` and `dogstatsd-uds/Bytes` expvar.

- Adds the ability to use `DD_TAGS` to set global tags in Fargate.

- Added a support for the new pod log directory pattern introduced in version 1.14 of Kubernetes to make sure
  the agent keeps on collecting logs after upgrade of a Kubernetes cluster.


.. _Release Notes_6.13.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add a kube_cronjob tag in the tagger. It applies to container metrics, autodiscovery metrics and logs.

- Change the prefix of entity IDs to make it easier to query the tagger
  without knowing what the container runtime is.

- APM: reduce memory usage in high traffic by up to 10x.

- APM: Services are no longer aggreagated in the agent, nor written to the Datadog API.
  Instead, they are now automatically extracted on the backend based on the received
  traces.

- APM: The default interval at which the agent watches its resource usage has
  been reduced from 20s to 10s.

- APM: Improved processing concurrency and as a result, CPU usage decreased
  by 20% in some scenarios.

- APM: Queued sender was rewritten to improve performance around scenarios where network problems are present.

- APM: Code clean up around configuration and writer.

- The `datadog-agent version` command now prints the version of Golang the
  agent was compiled with.

- Display Go version in output of status command

- Upgraded JMXFetch to 0.30.0. See https://github.com/DataDog/jmxfetch/releases/tag/0.30.0

- APM: the trace agent now lets through a wider variety of traces, automatically correcting some malformed traces
  instead of dropping them. The following fields are now replaced with reasonable defaults if invalid or empty
  and truncated if exceeding max length: `span.service`, `span.name`, `span.resource`, `span.type`.
  `span.duration=0` is now allowed. Missing span start date now defaults to `duration - now`. The
  `datadog.trace_agent.receiver.traces_dropped` metric is now tagged with a `reason` tag explaining the reason
  it was dropped. There is a new `datadog.trace_agent.receiver.spans_malformed` metric also tagged by `reason`
  explaining how the span was malformed.

- Refactored permissions check in the integration command.

- Support Python 3 for the integration command.


.. _Release Notes_6.13.0_Deprecation Notes:

Deprecation Notes
-----------------

- APM: The presampler has been rebranded as a "rate limiter" to avoid confusing it
  with other sampling mechanisms.

- APM: The "datadog.trace_agent.presampler_rate" metric has been deprecated in favor
  of "datadog.trace_agent.receiver.ratelimit".


.. _Release Notes_6.13.0_Security Issues:

Security Issues
---------------

- On Windows, quote the service name when registering service.  Mitigates
  CVE-2014-5455. Note that since the Agent is not running as admin, even
  a successful attack would not give admin rights as specified in the CVE.


.. _Release Notes_6.13.0_Bug Fixes:

Bug Fixes
---------

- Fix the `tagger` behavior returning `None` when no tags are present for the `kubelet` and `fargate` integration.

- APM: metrics generated by the processing function (such as *.traces_priority) now
  contain language specific tags.

- APM: Memory spikes when retry queue grows have been fixed.

- Fix 'vcruntime140.dll is being held in use by the following process'

- System-probe s6 services: ensure that the system-probe binary is bundled
  before trying to run it / stop it.
  This is to ensure that the s6-services definitions will be backward compatible
  with older builds that didn't have the system-probe yet.

- Fix a bug in the log scanning logic of the JMXFetch wrapper that would make
  JMXFetch hang if it logged a very large log entry

- Fixed an issue where logs collected from kubernetes using '/var/log/pods' would show up with a wrong format '{"log":"x","stream":"y","time":"z"}' on the logs explorer when using docker as container runtime.

- Fix TLS connection handshake that hang forever making the whole logs
  pipeline to be stucked resulting in logs not being tailed and file
  descriptor not being closed.

- On Windows, fixes bug in which Agent can't start if the Go runtime can't
  determine the ddagentuser's profile directory.  This information isn't
  used, so shouldn't cause a failure

- The External Metrics Setter no longer stops trying to get metrics after 3 failed attempts. Instead, it will retry indefinitely.

- Removes an unused duplicate copy of the ``system-probe`` binary from the Linux packages

- The NTP check now properly uses the ``port`` configuration option.


.. _Release Notes_6.13.0_Other Notes:

Other Notes
-----------

- Logs informing about check runs and payload submission are now displayed once
  every 500 events instead of every 20 events.


6.12.2
======

Prelude
-------

Release on: 2019-07-03

This release is only available on Windows and contains all the changes introduced in 6.12.0 and 6.12.1.

- Please refer to the `6.12.2 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6122>`_ for the list of changes on the Core Checks

6.12.1
======

Prelude
-------

Release on: 2019-06-28

This release is not available on Windows.

- Please refer to the `6.12.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6121>`_ for the list of changes on the Core Checks

Bug Fixes
---------

- Fixed a bug in the kubelet and fargate integrations preventing the collection of the ``kubernetes.cpu.*`` and ``kubernetes.memory.*`` metrics.

6.12.0
======

Known Issues
-------

Some metrics from the kubelet and fargate integrations (``kubernetes.cpu.*`` and ``kubernetes.memory.*``) are missing for certain configurations.
A fix will be released in v6.12.1. Meanwhile if downgrading to 6.11.3 is not an option we recommend using the runtime metrics
(ex: ``docker.cpu.*``, ``docker.mem.*``, ``containerd.cpu.*``, ...).

Prelude
-------

Release on: 2019-06-26

This release is not available on Windows.

- Please refer to the `6.12.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6120>`_ for the list of changes on the Core Checks


Upgrade Notes
-------------

- APM: Log throttling is now automatically enabled by default when
  `log_level` differs from `debug`. A maximum of no more than 10 error
  messages every 10 seconds will be displayed. If you had it enabled before,
    it can now be removed from the config file.

- On Windows, the path of the embedded ``python.exe`` binary has changed from
  ``%ProgramFiles%\Datadog\Datadog Agent\embedded\python.exe`` to ``%ProgramFiles%\Datadog\Datadog Agent\embedded2\python.exe``.
  If you use this path from your provisioning scripts, please update it accordingly.
  Note: on Windows, to call the embedded pip directly, please use ``%ProgramFiles%\Datadog\Datadog Agent\embedded2\python.exe -m pip``.

- Logs: Breaking Change for Kubernetes log collection - In the version 6.11.2 logic was added in the Agent to first look for K8s container files if `/var/log/pods` was not available and then to go for the Docker socket.
  This created some permission issues as `/var/log/pods` can be a symlink in some configuration and the Agent also needed access to the symlink directory.

  This logic is reverted to its prior behaviour which prioritise the Docker socket for container log collection.
  It is still possible to force the agent to go for the K8s log files even if the Docker socket is mounted by using the `logs_config.k8s_container_use_file' or `DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE`. parameter.
  This is recommended when more than 10 containers are running on the same pod.


New Features
------------

- A count named ``datadog.agent.started`` is now sent with a value of 1 when the agent starts.

- APM: Maximum allowed CPU percentage usage is now
  configurable via DD_APM_MAX_CPU_PERCENT.

- Node Agent can now perform checks on kubernetes service endpoints.
  It consumes the check configs from the Cluster Agent API via the
  endpointschecks config provider.
  Versions 1.3.0+ of the Cluster Agent are required for this feature.

- Logs can now be collected from init and stopped containers (possibly short-lived).

- Allow tracking pod labels and annotations value change to update labels/annotations_as_tags.
  Make the explicit tagging feature dynamic (introduced in https://github.com/DataDog/datadog-agent/pull/3024).


Enhancement Notes
-----------------

- APM: the writer will now flush based on an estimated number of bytes
  in accumulated buffer size, as opposed to a maximum number of spans,
  resulting in better traffic and payload size management.

- APM: traces are not dropped anymore because or rate limiting due to
  performance issues. Instead, the trace is kept in a queue awaiting to
  be processed.

- Logs docker container ID when parse invalid docker log in DEBUG level.

- Set the User-Agent string to include the agent name and version string.

- Adds host tags in the Hostname section of the
  agent status command and the status tab of the GUI.

- Expose the number of logs processed and sent to the agent status

- Added a warning message on agent status command and status gui
  tab when ntp offset is too large and may result in metrics
  ignored by Datadog.

- APM: minor improvements to CPU performance.

- APM: improved trace writer performance by introducing concurrent writing.

- APM: the stats writer now writes concurrently to the Datadog API, improving resource usage and processing speed of the trace-agent.

- Extends the docker check to accommodate the kernel memory usage metric.
  This metric shows the cgroup current kernel memory allocation.

- Ask confirmation before overwriting the output file while using
  the dogstatsd-stats command.

- Do not ship autotools within the Agent package.

- The ``datadog-agent integration`` subcommand is now capable of installing prereleases of official integration wheels

- Upgraded JMXFetch to 0.29.1. See https://github.com/DataDog/jmxfetch/releases/tag/0.28.0,
  https://github.com/DataDog/jmxfetch/releases/tag/0.29.0 and
  https://github.com/DataDog/jmxfetch/releases/tag/0.29.1

- Added validity checks to NTP responses

- Allow the '--check_period' flag of jmxfetch to be overriden by the
  DD_JMX_CHECK_PERIOD environment variable.

- Ship integrations and their dependencies on Python 3 in Omnibus.

- Added a warning about unknown keys in datadog.yaml.


Deprecation Notes
-----------------

- APM: the yaml setting `apm_config.trace_writer.max_spans_per_payload`
  is no longer in use; writes are now based solely on accumulated byte
  size.


Bug Fixes
---------

- Updated the DataDog/gopsutil library to include changes related to excessive DEBUG logging in the process agent

- The computeMem is only called in the check when we ensure that it does not get passed with an empty pointer.
  But if someone was to reuse it without checking for the nil pointer it could cause a segfault.
  This PR moves the nil checking logic inside the function to ensure it is safe.

- APM: Fixed a bug where normalize tag would not truncate tags correctly
  in some situations.

- APM: Fixed a small issue with normalizing tags that contained the
  unicode replacement character.

- APM: fixed a bug where modulo operators caused SQL obfuscation to fail.

- Fix issue on process agent for DD_PROCESS_AGENT_ENABLED where 'false' did not turn off process/container collection.

- Fix an error when adding a custom check config through the GUI
  when the folder where the config will reside does not
  exist yet.

- APM: on macOS, trace-agent is now enabled by default, and, similarly to other
  platforms, can be enabled/disabled with the `apm_config.enabled` config setting
  or the `DD_APM_ENABLED` env var

- Fix a bug where when the log agent is mis-configured, it temporarily hog on resources after being killed

- Fix a potential crash when doing a ``configcheck`` while the agent was not properly initialized yet.

- Fix a crash that could occur when having trouble connecting to the Kubelet.

- Fix nil pointer access for container without memory cgroups.

- Improved credentials scrubbing logic.

- The ``datadog-agent integration show`` subcommand now properly accepts only Datadog integrations as argument

- Fix incorrectly reported IO metrics when OS counters wrap in Linux.

- Fixed JMXFetch process not being terminated on Windows in certain cases.

- Empty logs could appear when collecting Docker logs in addition
  to the actual container logs. This was due to the way the Agent
  handles the header Docker adds to the logs. The process has been
  changed to make sure that no empty logs are generated.

- Fix bug when docker container terminate the last logs are missing
  and partially recovered from restart.

- Properly move configuration files for wheels installed locally via the ``integration`` command.

- Reduced memory usage of the flare command

- Use a custom patch for a costly regex in PyYAML,
  see `<https://github.com/yaml/pyyaml/pull/301>`_.

- On Windows, restore the ``system.mem.pagefile.pct_free`` metric


Other Notes
-----------

- The 'integration freeze' cli subcommand now only
  displays datadog packages instead of the complete
  result of the 'pip freeze' command.
- The Secrets Management feature is no longer in beta.


.. _Release Notes_6.11.3:

6.11.3
======

.. _Release Notes_6.11.3_Prelude:

Prelude
-------

Release on: 2019-06-04

- Please refer to the `6.11.3 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.11.3>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.11.3_Upgrade Notes:

Upgrade Notes
-------------

- Upgrade JMXFetch to 0.27.1


.. _Release Notes_6.11.3_Bug Fixes:

Bug Fixes
---------

- APM: fixed a bug where secrets in environment variables were ignored.

.. _Release Notes_6.11.2:

6.11.2
======

.. _Release Notes_6.11.2_Prelude:

Prelude
-------

Release on: 2019-05-23

.. _Release Notes_6.11.2_Enhancement Notes:

Enhancement Notes
-----------------

- Add option `cf_os_hostname_aliasing` to send the OS hostname as an alias when using the BOSH agent on Cloud Foundry.


.. _Release Notes_6.11.2_Bug Fixes:

Bug Fixes
---------

- Fixes problem in which Windows Agent wouldn't install on non-English machines due to assumption that "Performance Monitor Users" didn't need to be localized.
- Windows Installer is now more resilient to missing domain controller.

.. _Release Notes_6.11.1:

6.11.1
======

**Important**: ``6.11.1`` is not marked as latest for Windows: we are
investigating some cases where ``6.11.0`` and ``6.11.1`` are not installing correctly
on Windows.
Downloading ``datadog-agent-6-latest.amd64.msi`` will give you version ``6.10.1``.

.. _Release Notes_6.11.1_Prelude:

Prelude
-------

Release on: 2019-05-06

- Please refer to the `6.11.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6110>`_ for the list of changes on the Core Checks.
- Please refer to the `6.11.1 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.11.1>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.11.1_Upgrade Notes:

Upgrade Notes
-------------

- Change the prioritization between the two logic that we have to collect logs on Kubernetes.
  Now attempt first to collect logs on '/var/log/pods' and fallback to using the docker socket if the initialization failed.

.. _Release Notes_6.11.1_Bug Fixes:

Bug Fixes
---------

- Fix a bug where short image name wouldn't be properly set on old docker versions
- Properly handle docker container logs in multiline mode in case of infrequence log messages, log file rotations or agent restart


.. _Release Notes_6.11.0:

6.11.0
======

.. _Release Notes_6.11.0_Prelude:

Prelude
-------

Release on: 2019-04-17

- Please refer to the `6.11.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6110>`_ for the list of changes on the Core Checks.

- Please refer to the `6.11.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.11.0>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.11.0_Upgrade Notes:

Upgrade Notes
-------------

- APM: move flush notifications from level "INFO" to "DEBUG"

- APM: logging format has been changed to match the format of the core agent.

- Metrics coming through dogstatsd with the following internal prefixes: ``activemq``, ``activemq_58``,
  ``cassandra``, ``jvm``, ``presto``, ``solr``, ``tomcat``, ``kafka``, ``datadog.trace_agent``,
  ``datadog.process``, ``datadog.agent``, ``datadog.dogstatsd`` are no longer affected by the
  ``statsd_metric_namespace`` option.

- Removed the internal ability to send logs to a specific logset at agent level.

- On Windows, the Datadog Agent now runs as a non-privileged user
  (ddagentuser by default) rather than LOCAL_SYSTEM. Please refer to our `dedicated docs <https://docs.datadoghq.com/agent/faq/windows-agent-ddagent-user/>`_ for more information

- The Windows installer will no longer allow direct downgrades; if
  a downgrade is required, the user must uninstall the newer version
  and install the older version.


.. _Release Notes_6.11.0_New Features:

New Features
------------

- Secrets beta feature is now available on windows allowing users to pull
  secrets from secret management services.

- APM: JSON logging is now supported using the `log_format_json: true` setting.

- Collect container thread count and thread limit

- JMXFetch upgraded to 0.27.0. See `0.27.0  <https://github.com/DataDog/jmxfetch/releases/tag/0.27.0>`_ for more details.

- The agent now ignores pod that exited more than 15 minutes ago to
  reduce its resource footprint when pods are not garbage-collected.
  This is configurable with the kubernetes_pod_expiration_duration option.

- Now support CRI-O container runtime for log collection on Kubernetes.

- Automatically add a "dirname" tag representing the directory of logs tailed from a wildcard path.


.. _Release Notes_6.11.0_Enhancement Notes:

Enhancement Notes
-----------------

- AutoDiscovery can now monitor unready pods.
  It looks for a new pod annotation "ad.datadoghq.com/tolerate-unready"
  which, if set to `true` will make AutoDiscovery monitor that pod
  regardless of its readiness state.

- Add the ability for the ``datadog-agent check`` command to have Python checks start
  an `interactive debugging session <https://docs.python.org/2/library/pdb.html>`_.

- Change the logging format to include the name of the logging agent instead of appending it in the agent container logs.

- Add /metrics to the bare endpoints the agent can access.
  This is required to support querying endpoints protected by
  RBAC, by kube-rbac-proxy for instance.

- APM: errors reported by the receiver's HTTP server are now
  shown in the logs.

- APM: slightly improved normalization error logs.

- On Windows, allows Agent to be installed to nonstandard directories.
  Uses APPLICATIONDATADIRECTORY to set the root of the configuration file tree,
  and PROJECTLOCATION to set the root of the binary tree. Please refer to
  the `docs <https://docs.datadoghq.com/agent/basic_agent_usage/windows>`_
  for more details

- In order to decrease the number of API DCA requests,
  The Agent now uses a different API endpoint to call
  the DCA's API only once in order to retrieve the Pods
  metadata.

- Host metadata payloads are now zlib-compressed

- Log file size and number of rotation is now configurable.

- Add a command `dogstatsd-stats` to the agent to get
  basic stats about the processed metrics.

- Support JSON arrays within environment variables, in addition to space separated
  values.

- On Google Compute Engine, the Agent now reports `<instance_name>.<project_id>`
  as a host alias instead of `<hostname_prefix>.<prefix_id>`, which improves the
  uniqueness and relevance of the host alias when the GCE instance has a custom hostname.

- The import command doesn't stop anymore when there is no ``conf.d`` or
  ``auto_conf`` directory.

- Kubernetes event collection timeout can now be configured.

- Improve status page by splitting errors and warnings from the Logs agent

- Secrets are no longer decrypted in agent command when it's not needed
  (commands like hostname, launchgui, configuration ...). This reduce the
  number of times the 'secret_command_backend' executable will be called.

- Improved memory efficiency on hosts sending very high numbers of metrics.

- Resolve once the DNS name given by docker and try the associated IP to reach the kubelet.
  Prioritize HTTPS over HTTP to connect to kubelet.
  Prioritize communication using IPs over hostnames to spare DNS servers accross the cluster.


.. _Release Notes_6.11.0_Deprecation Notes:

Deprecation Notes
-----------------

- Removal of largely unused go SNMP check. SNMP support still
  provided by the python variant.


.. _Release Notes_6.11.0_Bug Fixes:

Bug Fixes
---------

- Fix an auto-discovery annotation value parsing limitation in version 6
  compared to version 5.
  Now, ``ad.datadoghq.com/*.instances`` annotation key supports value like ``[[{"foo":"bar1"}, {"foo":"bar2"}], {"name":"bar3"}]``

- The agent container will now output valid JSON when using JSON log format.

- APM: Multiple value "Content-Type" headers are now parsed correctly
  for media type in the HTTP receiver.

- APM: always reply with correct Content-Type in API responses.

- APM: when a span's resource is empty, the error "`Resource` can not be empty"
  will be returned instead of the wrong "`Resource` is invalid UTF-8".

- APM: sensitive information is now scrubbed from logs.

- APM: Fix issue with `--version` flag when API key is unset.

- APM: Ensure UTF-8 characters are not cut mid-way when truncating
  span fields.

- Metrics coming through dogstatsd with the following internal prefixes: ``activemq``, ``activemq_58``,
  ``cassandra``, ``jvm`, ``presto``, ``solr``, ``tomcat``, ``kafka``, ``datadog.trace_agent``,
  ``datadog.process``, ``datadog.agent``, ``datadog.dogstatsd`` are no longer affected by the
  ``statsd_metric_namespace`` option.

- Fixes ec2 tags collection when datadog agent is deployed
  into a kubernetes cluster along with kube2iam.

- Fixes bug in which upgrading from agent5 doesn't correctly import the configuration

- Fix a race condition in gohai that could make the Agent crash while collecting
  the host's filesystem metadata

- Hostnames containing characters that are invalid for a filename no longer prevent the agent
  from generating a flare.

- Allow macOS users to invoke the `datadog-agent integration` command as root since the installation directory is owned by root.

- Change to a randomized exponential backoff in case of connection failure

- Ignore empty logs_dd_url to fall back on default config for logs agent.

- Detect and handle Docker logs with only header and empty content

- To mitigate issues with the hostname detection on AKS, hostnames gathered from
  the metadata endpoints of AWS, GCE, Azure, and Alibaba cloud are no longer considered
  valid if their length exceeds 255 characters.


.. _Release Notes_6.11.0_Other Notes:

Other Notes
-----------

- Bump embedded Python to 2.7.16


6.10.2
======

Prelude
-------

Release on: 2019-03-20


Bug Fixes
---------

- Fix a race condition in Autodiscovery leading to some checks not
  being unscheduled on container exit

.. _Release Notes_6.10.1:

6.10.1
======

.. _Release Notes_6.10.1_Prelude:

Prelude
-------

Release on: 2019-03-07


.. _Release Notes_6.10.1_Bug Fixes:

Bug Fixes
---------

- APM: Mixing cases in `apm_config.analyzed_spans` and `apm_config.analyzed_rate_by_service`
  entries is now allowed. Service names and operation names will be treated as case insensitive.

- Refactor the ``ContainerdUtil`` so that each call to the ``containerd`` api has a dedicated timeout.


.. _Release Notes_6.10.0:

6.10.0
======

.. _Release Notes_6.10.0_Prelude:

Prelude
-------

Release on: 2019-02-28

- Please refer to the `6.10.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-6100>`_ for the list of changes on the Core Checks.

- Please refer to the `6.10.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.10.0>`_ for the list of changes on the Process Agent.

- Starting with this release, the changes on the Trace Agent are listed in the present release notes.


.. _Release Notes_6.10.0_Security Notes:

Security Notes
--------------

- The Agent now defaults to aliasing `yaml.load` and `yaml.dump` to `yaml.safe_load` and `yaml.safe_dump` for ALL checks
  as a defense-in-depth measure against CVE-2017-18342. The Datadog Agent does not use the vulnerable code directly. The
  effort to patch the PyYAML library guards against the accidental unsafe use of this library by custom checks and transitive
  dependencies. Specifically, the kubernetes client library v8.0.1 calls the unsafe `yaml.load` function, but the fix provided
  forces the use of `yaml.safe_load` by default. In this release of the Agent, kubernetes client library v8.0.1 is only used
  by the new `kube_controller_manager` integration. If for any reason you encounter problems with your custom checks, please
  reach out to support.


.. _Release Notes_6.10.0_New Features:

New Features
------------

- Introduce pod and container tagging through annotations.

- Docker images are now signed with Content Trust to ensure their integrity when pulling

- Dogstatsd can now inject extra tags on a metric when a special entity tag is provided

- ``datadog-agent integration install`` command allows to install a check from a locally available wheel (.whl file)
  with the added parameter ``--local-wheel``.

- JMXFetch upgraded to 0.26.1: introduces concurrent metric collection across
  JMXFetch check instances. Concurrent collection should avoid stalling
  healthy instances in the event of issues (networking, system) in
  any of the remaining instances configured. A timeout of ``jmx_collection_timeout``
  (default 60s) is enforced on the whole metric collection run.
  See `0.25.0  <https://github.com/DataDog/jmxfetch/releases/tag/0.25.0>`_,
  `0.26.0  <https://github.com/DataDog/jmxfetch/releases/tag/0.26.0>`_ and
  `0.26.1  <https://github.com/DataDog/jmxfetch/releases/tag/0.26.1>`_.

- Added the possibility to define global logs processing rules in `datadog.yaml` that will be applied to all logs,
  in addition to integration logs processing rules when defined.


.. _Release Notes_6.10.0_Enhancement Notes:

Enhancement Notes
-----------------

- Consider static pods as ready, even though their status is never updated in the pod list.
  This creates the risk of running checks against pods that are not actually ready, but this
  is necessary to make autodiscovery work on static pods (which are used in standard kops
  deployments for example).

- Adds the device mapper logical volume name as a tag
  in the system.io infos.

- Extends the docker check to accommodate the failed memory count metric.
  This metric increments every time a cgroup hits its memory limit

- Add a ``--json`` flag to the ``check`` command that will
  output all aggregator data as JSON.

- [tagger] Add pod phase to kubelet collector

- The Agent logs now contains the relative file path (including the package) instead of only the filename.

- Each corecheck could now send custom tags using
  the ``tags`` field in its configuration file.

- ECS: running the agent in awsvpc mode is now supported, provided it runs in a
  security group that can reach both the containers to monitor and the host via
  its private IP on port 51678

- The performance of the Agent under DogStatsD load has been improved.

- Improve memory usage when metrics, service checks or events contain many tags.

- APM: improve performance of NormalizeTag function.

- Use dedicated ``datadog_checks_downloader`` to securely download integrations wheels when using the ``datadog-agent integration install`` command.

- A warning is now displayed in the status when the connection to the
  log endpoint cannot be established

- When shutting the agent down, cancel ongoing python subprocess
  so they can exit as cleanly and gracefully as possible.

- Add of a "secrets" command to show information about decrypted secrets. We
  now also track the configuration's name where each secrets was found.

- Secrets are now resolved in environment variables.

- In order to ensure compatibility with systemd < 229,
  ``StartLimitBurst`` and ``StartLimitInterval`` have been
  moved to the Service section of the service files.

- Files are not tailed in reverse lexicographical order w.r.t their file names then dir name. If you have files
  `/1/2017.log`, `/1/2018.log`, `/2/2018.log` and `logs_config.open_files_limit == 2`, then you will tail
  `/2/2018.log` and `/1/2018.log`.

- Include ``.yml`` configuration files in the flare.


.. _Release Notes_6.10.0_Bug Fixes:

Bug Fixes
---------

- Fix an issue where some auto-discovered integrations would not get rescheduled when the template was not containing variables

- Autodiscovery now removes children configurations when removing templates

- Fix the display of unresolved configs in the verbose output of the ``configcheck`` command

- Fix custom command line port configuration on `configcheck` and `tagger-list` CLI commands.

- When the secrets feature is enabled, fix bug preventing the ``additional_endpoints``
  config option from being read correctly

- Fix "status" command JSON output to exclude non JSON header. The output of
  the command is now a valid JSON payload.

- APM: Fix a potential memory leak problem when the trace agent is stopped.

- Fixed a bug where logs forwarded by UDP would not be split because of missing line feed character at the end of a datagram.
  Now adding a line feed character at the end of each frame is deprecated because it is automatically added by the agent on read operations.

- Fix an issue where some kubernetes tags would not be properly removed.


.. _Release Notes_6.10.0_Other Notes:

Other Notes
-----------

- The Agent is now compiled with Go 1.11.5

- Custom checks default on safe pyyaml methods.


.. _Release Notes_6.9.0:

6.9.0
=====

.. _Release Notes_6.9.0_Prelude:

Prelude
-------

Release on: 2019-01-22

- Please refer to the `6.9.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-690>`_ for the list of changes on the Core Checks.

- Please refer to the `6.9.0 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.9.0>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.9.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.9.0>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.9.0_Upgrade Notes:

Upgrade Notes
-------------

- On EC2 hosts that were upgraded from Agent 5.x using the install script or that have the ``hostname_fqdn`` option enabled if your hostname currently begins with ``ip-`` or ``domU-`` (default EC2 hostnames) your hostname will change to the EC2 instance ID.
  Example: ``ip-10-1-1-1.ec2.internal`` => ``i-1234567890abcdef0``.
  This is an effort to fix a bug in the hostname resolution that was introduced in the version 6.3 of the Agent.

- Kubernetes logs integration is now automatically enabled if it can find ``/var/log/pods``.
  If ``logs_config.container_collect_all`` is not enabled, only pods with Datadog logs
  annotation will be collected. If ``logs_config.container_collect_all`` is enabled, logs for
  all pods (matching ``ac_exclude`` and ``ac_include`` filters if applicable) will be collected.


.. _Release Notes_6.9.0_New Features:

New Features
------------

- Introduce a way to configure the cardinality level of tags that
  the tagger should return. This is split between two options - one for
  checks and one for dogstatsd. The three cardinality levels are High,
  Orchestrator, and Low. Checks get Low and Orchestrator-level tags by default
  Dogstatsd benefits from Low-card tags only by default.

- You can add extra listeners and config providers via the ``DD_EXTRA_LISTENERS`` and
  ``DD_EXTRA_CONFIG_PROVIDERS`` enviroment variables. They will be added on top of the
  ones defined in the ``listeners`` and ``config_providers`` section of the datadog.yaml
  configuration file.

- Adding native containerd check, based on the containerd socket.

- You can now see an extra instance id when displaying the Agent status depending on the check.
  If the instance contains an attribute ``name`` or ``namespace``, it will be displayed next to the instance id.

- Added a new ``container_cgroup_prefix`` option to fix some cases where system slices
  were detected as containers.

- Add ``datadog-agent integration show [package]`` command to show information about an installed integration.


.. _Release Notes_6.9.0_Enhancement Notes:

Enhancement Notes
-----------------

- AutoDiscovery can now monitor unready pods.
  It looks for a new pod annotation "ad.datadoghq.com/tolerate-unready"
  which, if set to `true` will make AutoDiscovery monitor that pod
  regardless of its readiness state.

- Add debug information about the secrets feature to the flare.

- On the ``check`` command, add a pause of 1sec between the 2 check runs when
  ``--check-rate`` is set. Allows some checks to gather more meaningful metric samples.

- Docker disk IO metrics are now tagged by ``device``

- Introduces an expvar reporting the number of dogstatsd
  packets per second processed if `dogstatsd_stats_enable`
  is enabled.

- Add an Endpoints section in the GUI status page and the
  CLI status command, listing all endpoints used by the agent
  and their api keys.

- Expose number of packets received for each dogstatsd listeners through expvar

- Better descriptions of the ``install`` and ``freeze`` subcommands of the ``datadog-agent integration`` command.

- In the flare, try to redact api keys from other services.

- Support the ``site`` config option in the log agent.

- Add ability for Python checks to submit trace logs.


.. _Release Notes_6.9.0_Bug Fixes:

Bug Fixes
---------

- datadog/dogstatsd image: gohai metadata collection is now disabled by default

- If `dogstatsd_stats_enable` is indeed enabled, we should
  consume and report on the generated stats. Fixes stagnant
  channel and misleading debug statement.

- Fix a hostname resolution bug on EC2 hosts which could happen when the ``hostname_fqdn`` option was enabled, and which made the Agent use a non-unique FQDN instead of the EC2 instance ID.

- Fix a bug with parsing of ``trace.ignore`` in the ``import`` command.

- Fixes bug in windows core checks where adding/removing devices isn't
  caught, so only devices present on startup are monitored.

- Fix bug of the ``datadog-agent integration install`` command that prevented
  moving configuration files when the ``conf.d`` folder is a mounted directory.

- The ``datadog-agent integration install`` command creates the configuration folder
  for an integration with the correct permissions so that the configuration files can be copied.

- On windows, fixes downgrades.  Fix won't be apparent for an
  additional release, since the core fix occurs on install.

- On Windows, further fixes when installation drive isn't c:.  Fixes
  problem where `logs` was effectively hardcoded to use `c:` for programdata
  Fixes installation problem where process & trace service were using
  `c:\programdata\...` to find datadog.yaml regardless of installation dir

  If upgrading from a prior version, the configuration file (datadog.yaml) may
  have incorrect data.  It will be necessary to manually update those entries.
  For example
  `confd_path: c:\programdata\datadog\conf.d`
  will have to be changed to
  `confd_path: d:\programdata\datadog\conf.d`
  etc.

- Removed the command arguments from the flare's container list
  to avoid collecting sensitive information

- Fix a rare crash caused by a nil map dereference in the ``gohai`` library

- Reintroducing JMXFetch process lifecycle management on Linux.
  Adding JMXFetch healthcheck for docker environments.

- Fix warning about unknown setting "StartLimitIntervalSecs" in the agent
  service file with systemd version <=229.


.. _Release Notes_6.9.0_Other Notes:

Other Notes
-----------

- The ``datadog-agent integration`` command is now GA.

- On the packaged Linux Agent, the python interpreter is now built with the
  ``-fPIC`` flag.

- JMXFetch upgraded to 0.24.1. See https://github.com/DataDog/jmxfetch/releases/tag/0.24.0 and
  https://github.com/DataDog/jmxfetch/releases/tag/0.24.1

- Log host metadata at debug level regardless of its size.

.. _Release Notes_6.8.3:

6.8.3
=====

.. _Release Notes_6.8.3_Prelude:

Prelude
-------

Release on: 2018-12-27

.. _Release Notes_6.8.3_Bug Fixes:

Bug Fixes
---------

- Fix a bug that could send the last log multiple times when a container was not writing
  new logs

- Fix a panic that could occur if a container doesn't send logs for more than 30 sec and
  the timestamp of the last received log is corrupted

.. _Release Notes_6.8.2:

6.8.2
=====

.. _Release Notes_6.8.2_Prelude:

Prelude
-------

Release on: 2018-12-19

.. _Release Notes_6.8.2_Bug Fixes:

Bug Fixes
---------

- Fix a panic that could occur when a container stopped while the agent was reading logs from it.

.. _Release Notes_6.8.1:

6.8.1
=====

.. _Release Notes_6.8.1_Prelude:

Prelude
-------

This is a container only release that fixes a bug introduced in ``6.8.0`` that was impacting the kubernetes integration.

Release on: 2018-12-17

- Please refer to the `6.8.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-681>`_ for the list of changes on the Core Checks.

.. _Release Notes_6.8.1_Bug Fixes:

Bug Fixes
---------

- Fixes the default ``kubelet`` check configuration that was preventing the kubernetes integration from working properly

.. _Release Notes_6.8.0:

6.8.0
=====

.. _Release Notes_6.8.0_Prelude:

Prelude
-------

Please note that a critical bug has been identified in this release that would prevent the kubernetes integration from collecting kubelet metrics on containerized agents.
The severity of the issue has led us to remove the ``6.8.0`` images on dockerhub and to make the ``latest`` tag point to the ``6.7.0`` release.
If you have upgraded to this version of the containerized agent we recommend you downgrade to ``6.7.0``. Linux packages are not affected.

Release on: 2018-12-13

- Please refer to the `6.8.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-680>`_ for the list of changes on the Core Checks.

- Please refer to the `6.8.0 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.8.0>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.8.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.8.0>`_ for the list of changes on the Process Agent.

The Datadog Agent now automatically look for the container short image name to set the default value for the log source and service.
The source is especially important as it triggers the **automatic configuration of your platform with integration pipeline and facets**.
The Datadog Agent autodiscovery can still be used to override the default source and service with pod annotations or container labels.

Upgrade Notes
-------------

- The agent now requires a cluster agent version 1.0+ to establish
  a valid connection
- JMX garbage collection metrics ``jvm.gc.cms.count`` and ``jvm.gc.parnew.time`` were renamed to ``jvm.gc.minor_collection_count``, ``jvm.gc.major_collection_count``, ``jvm.gc.minor_collection_time``, ``jvm.gc.major_collection_time`` in 6.6 to be more meaningful. To ensure backward compatibility the change was reverted in this release and the new names put behind a config option. If you started relying on these new names please enable the ``new_gc_metrics`` option in your jmx configurations. An example can be found `here <https://github.com/DataDog/datadog-agent/blob/1aee233a18dedbb8af86da0ce1f2e305206aacf8/cmd/agent/dist/conf.d/jmx.d/conf.yaml.example#L8-L13>`_. This flag will be enabled by default in a future major release.

New Features
------------

- Enable docker config provider if docker.sock exists

- The new command ``datadog-agent config`` prints the runtime config of the
  agent.

- Adds eBPF-based network collection component called network-tracer.

- Add diagnosis to the agent for connectivity to the cluster agent

- ``datadog-agent integration install`` command prevents a user from downgrading an integration
  to a version older than the one shipped by default in the agent.

- Adding kerberos support with libkrb5.

- ``datadog-agent integration install`` command moves configuration files present in
  the ``data`` directory of the wheel upon successful installation


Enhancement Notes
-----------------

- Adding a default location on Windows for the file storing pointers to make sure we never lose nor duplicate any logs

- Add an option to the `agent check` command to run the check n times

- Set service and source to the docker short image name when container_collect_all flag
  is enabled and no label or annotation is defined

- Docker: the datadog/dogstatsd image now ships a healthcheck

- Improved consistency of the ECS and Fargate tagging

- Improve logging when python checks use invalid types for tags

- Added a ``region`` tag to Fargate containers, indicating the AWS region
  they run in

- Adds system.cpu.interrupt, and system.mem.committed, system.mem.paged,
  system.mem.nonpaged, system.mem.cached metrics on Windows

- Add ``permissions.log`` file to the flare archive.

- Add an agent go-routine dump to the flare as reported
  by the built-in pprof runtime profiling interface.

- The agent can now expose its healthcheck on a dedicated http port.
  The Kubernetes daemonset uses this by defaut, on port 5555.

- It's possible now to have different poll intervals for
  each autodiscovery configuration providers

- Improve Windows Event parsing. Event.EventData.Data fields are parsed as one JSON object. Event.EventData.Binary field
  is parsed to its string value

- Rename the Windows Event "#text" field to "value". This fixes the facet
  creation of those fields

- Add a ``status.log`` and a ``config-check.log`` with a basic message in the flare
  if the agent is not running or is unreachable.

- Added support for wildcards to `DD_KUBERNETES_POD_LABELS_AS_TAGS`. For example,
  `DD_KUBERNETES_POD_LABELS_AS_TAGS='{"*":"kube_%%label%%"}'` will all pod labels as
  tags to your metrics with tags names prefixed by `kube_`.

Deprecation Notes
-----------------

- Removed support for logs_config.tcp_forward_port as it's no longer needed for other integrations.


Bug Fixes
---------

- Configure error log when failing to run docker inspect to read as debug instead, as this log is duplicated by the tagger.

- Fix a bug where `datadog-agent integration` users could not test the
  `--in-toto` flag due to a filesystem permission issue.

- The cluster agent client init now fails as expected if the
  cluster agent URL is not valid

- Print correct error when the ``datadog-agent integration`` command fails after installing an integration

- Fix build failure on 32bit armv7

- Fix a bug with Docker logging driver where logs would not be tailed after a log
  rotation when the option `--log-opt max-file=1` was set.

- Display the correct timezone name in the status page.

- On Windows, the agent now properly computes the location of ProgramData for
  configuration files instead of using hardcoded values


Other Notes
-----------

- JMXFetch upgraded to 0.23.0. See https://github.com/DataDog/jmxfetch/releases/tag/0.23.0

- On linux, use the cgo dns resolver instead of the golang one. The will make
  the agent use glibc to resolve hostnames and should give more predictable
  results.

- Starting with this Agent release, all the Datadog integrations that are installed
  with the ``datadog-agent integration install`` command are reset to their
  default versions when the Agent is upgraded.
  This guarantees the integrity of the embedded python environment after the upgrade.

- The ``datadog-agent integration`` command is now in Beta.

.. _Release Notes_6.7.0:

6.7.0
=====

.. _Release Notes_6.7.0_Prelude:

Prelude
-------

Release on: 2018-11-21

This release only ships changes to the trace-agent.

This release focuses on simplifying `Trace Search <https://docs.datadoghq.com/tracing/trace_search_and_analytics/>`_ configuration. APM Events can now be configured at the tracer level. Tracers will get updated in the near future to expose this option.

- Please refer to the `6.7.0 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.7.0>`_ for the list of changes on the Trace Agent.

.. _Release Notes_6.6.0:

6.6.0
=====

.. _Release Notes_6.6.0_Prelude:

Prelude
-------

Release on: 2018-10-25

- Please refer to the `6.6.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-660>`_ for the list of changes on the Core Checks.

- Please refer to the `6.6.0 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.6.0>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.6.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.6.0>`_ for the list of changes on the Process Agent.

.. _Release Notes_6.6.0_Known Issues:

Known Issues
------------

- JMX garbage collection metrics `jvm.gc.parnew.time` and `jvm.gc.cms.count` got renamed to `jvm.gc.minor_collection_time` and `jvm.gc.major_collection_count` on some JMX integrations. Since this change on the name of these 2 metrics may affect your dashboards and monitors, these metrics will also be sent under their older names in a later version of the Agent.

.. _Release Notes_6.6.0_New Features:

New Features
------------

- Disk check support for the puppy agent on unix-like systems

- Support for the upcoming cluster-agent cluster-level checks feature,
  via the ``clusterchecks`` config provider

- Add a new CRI core check that will send metrics about resource usage of your
  containers via the Container Runtime Interface.

- Support SysVinit on Debian
  note: some warnings can appear if you enable/disable the agent manually on a systemd system. They can be safely ignored

- The ``datadog-agent integration install`` command will now check for compatibility with ``datadog-checks-base``
  shipped with the agent. In case of mismatch, it will try to rollback to the previously installed integration
  version and exit with a failure.

- Add ``--in-toto`` flag to ``datadog-agent integration`` command to enable in-toto

- Add ``--verbose`` flag to ``datadog-agent integration`` command to enable verbose logging on pip and TUF

- Docker image: running with a read-only root filesystem is now supported


.. _Release Notes_6.6.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add a setting to configure the interval at which configs should be polled
  for autodiscovery.

- Support a new config option, ``site``, that allows setting the Datadog site
  to which the Agent should send data. ``dd_url`` is still supported and, when set,
  overrides ``site``.

- Display a warning in the agent status when too many logs are being tailed
  and the agent is not tailing them all. This happens with wildcards in path
  of the tailed files

- Dogstatsd supports removing the hostname on events and services checks as it did with metrics, by adding an empty ``host:`` tag

- Added new dogstatsd_tags variable which can be used to specify
  additional tags to append to all metrics received by dogstatsd.

- dogstatsd cleans up stale UNIX socket on startup.

- The ecs-agent's docker container name can now be set via the ``ecs_agent_container_name``
  option or the ``DD_ECS_AGENT_CONTAINER_NAME`` envvar for autodetection.

- EKS pause containers are ignored by default

- All python and go checks support the new ``empty_default_hostname`` option
  to send metrics with no hostname. This is used for cluster-level checks

- All go checks now support the ``min_collection_interval`` option, as python
  check already do

- Added a ``kubelet_wait_on_missing_container`` option to handle hosts where
  the kubelet's podlist is slow to update, leading to missing tags
  or failing Autodiscovery. Set it to 1 for a 1 second maximum wait

- Add an option to enable protobuf communication with the Kubernetes apiserver

- ``datadog-agent integration`` command will not pull any of the integration's dependencies

- More accurate tag extraction logic for Docker Swarm

- Added new command line properties to the Windows installer which allow for setting site specific configuration.


.. _Release Notes_6.6.0_Bug Fixes:

Bug Fixes
---------

- Fix an issue preventing the exit logs of the agent from displaying the correct filename.

- Fix bug that occurs when checks labels/annotation are misconfigured and would
  prevent the logs of the container to be tailed

- Fix an issue causing the agent to stop when systemd-journald service is stopped or fails

- Fix deadlock when an config item under ``logs`` is invalid

- Fix system.mem.pct_usable implementation on Linux 3.14+ to match Datadog Agent 5

- Fix a potential race in the autodiscovery where a service would be removed before
  its config could be resolved (causing the agent to crash)

- Fixes crash on Windows when the agent encounters a malformed performance counter database

- Fixes config.Digest that was not stable depending on the oder of tags in the instance.
  It also did not take into account LogsConfig, this is fixed as well.

- Fix an issue where the log agent would prevent files from being log rotated on Windows

- Correctly pass the agent's proxy settings to pip when using the ``datadog-agent integration`` command with TUF enabled.

- Recover from errors when connection to the docker socket is lost to continue tailing containers.

- When installing / updating wheels using the ``datadog-agent integration``
  command, we replace the PyPI index with our own by default, in order to
  prevent accidental installation of Datadog or even third-party packages
  from PyPI.

- Remove some undocumented power user options to the ``datadog-agent
  integration`` command to prevent accidental misconfiguration that may
  reduce security guarantees.


.. _Release Notes_6.6.0_Other Notes:

Other Notes
-----------

- JMXFetch upgraded to 0.21.0; Adds support for rmi registry connection over
  SSL and client authentication.

- Use autodiscovery in log-agent kubernetes integration


.. _Release Notes_6.5.2:

6.5.2
=====

.. _Release Notes_6.5.2_Prelude:

Prelude
-------

Release on: 2018-09-20

- Please refer to the `6.5.2 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-652>`_ for the list of changes on the Core Checks.

- Please refer to the `6.5.2 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.5.2>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.5.2 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.5.2>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.5.2_Bug Fixes:

Bug Fixes
---------

- Fix a crash in the logs package that could occur when a docker tailer initialization failed.


.. _Release Notes_6.5.1:

6.5.1
=====

.. _Release Notes_6.5.1_Prelude:

Prelude
-------

Release on: 2018-09-17

- Please refer to the `6.5.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-651>`_ for the list of changes on the Core Checks.

- Please refer to the `6.5.1 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.5.1>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.5.1 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.5.1>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.5.1_Bug Fixes:

Bug Fixes
---------

- Fix possible deadlocks that could occur when new docker sources
  and services are pushed and:

  * The docker socket is closed at agent setup
  * The docker socket is not mounted
  * The kubernetes integration is enabled

- Fix a deadlock that could occur when the logs-agent is enabled and the configuration
  parameter 'logs_config.container_collect_all' or the environment variable 'DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL' are set to true.


.. _Release Notes_6.5.0:

6.5.0
=====

.. _Release Notes_6.5.0_Prelude:

Prelude
-------

Released on: 2018-09-13

Please note that a critical bug identified in this release affecting container
log collection when the ``container_collect_all`` was set, would lead to an agent
deadlock. The severity of the issue has led us to remove the packages for the
affected platforms (**Linux** and **Docker**). If you have upgraded to this version,
on **Linux or Docker** we recommend you downgrade to ``6.4.2``.

- Please refer to the `6.5.0 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-650>`_ for the list of changes on the Core Checks.

- Please refer to the `6.5.0 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.5.0>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.5.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.5.0>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.5.0_New Features:

New Features
------------

- Autodiscovery: the ``docker`` and ``kubelet`` listeners will retry on error,
  to support starting the agent before your container runtime (host install)

- Bump the default number of check runners to 4. This has some
  concurrency implications as we will now run multiple checks in
  parallel.

- Kubernetes: to avoid hostname collisions between clusters, a new ``cluster_name`` option is available. It will be added as a suffix to the host alias detected from the kubelet in order to make these aliases unique across different clusters.

- Docker image: handle docker/kubernetes secret files with a helper script

- The Node Agent can rely on the Datadog Cluster Agent to collect Node Labels.

- Improved ECS fargate tagging:

  * Honor the ``docker_labels_as_tags`` option to extract custom tags
  * Make the ``cluster_name`` tag shorter
  * Add the ``short_image`` and ``container_id`` tags
  * Remove some noisy tags
  * Fix a lifecycle issue that caused missing tags

- The live containers view can now retrieve containers directly from the kubelet,
  in order to support containerd and crio

- Kubernetes events: setting event host tags to the related hosts, instead of the host collecting the events.

- Added dedicated configuration parameters to send logs to a proxy by TCP.
  Note that 'logs_config.dd_url', 'logs_config.dd_port' and 'logs_config.dev_mode_no_ssl' are deprecated and will be unvailable soon,
  use the new parameters 'logs_config.logs_dd_url' and 'logs_config.logs_no_ssl' instead.

- Added the possibility to send logs to Datadog using the port 443.


.. _Release Notes_6.5.0_Enhancement Notes:

Enhancement Notes
-----------------

- Add more environment variables to the flare whitelist

- When ``dd_url`` is set to ``app.datadoghq.eu``, the infra Agent also sends data
  to versioned endpoints (similar to ``app.datadoghq.com``)

- Make all numbers on the status page more human readable (using unit and SI prefix when appropriate)

- Display hostname provider and errors on the status page

- Kubelet Autodiscovery: reduce logging when no change is detected

- On Windows, the `hostname_fqdn` flag will now be honored, and the
  host reported by Datadog will be the fully qualified hostname.

- Enable all configuration options to be set with env vars

- Tags generated from GCE metadata may now be omitted by using
  ``collect_gce_tags`` configuration option.

- Introduction of a new bucketed scheduler to enable multiple
  check workers to increase concurrency while spreading the load
  over the collection interval.

- The 'status' command and 'status' page (in the GUI) now displays errors
  raised by the '__init__' method of a Python check.

- Exclude the rancher pause container in the agent

- On status page, allow users to know which instance of a check matches which yaml instance in configcheck page

- The file_handle check reports 4 new metrics for feature parity with agent 5

- The ntp check will now query multiple servers by default to be more
  resilient to servers returning wrong offsets. A now config option ``hosts``
  is now available in the ntp check configuration file to allow users to change
  the list of ntp servers.

- Tags and sources in the tagger-list command are now sorted to ease troubleshooting.

- To allow concurrent execution of subprocess calls from python, we now
  save the thread state and release the GIL to unblock the interpreter . We
  can reaquire the GIL and restore the thread state when the subprocess call
  returns.

- Add a new configuration option, named `tag_value_split_separator`, allowing the specified list of raw tags to have its value split by a given separator.
  Only applies to host tags, tags coming from container integrations. Does not apply to tags on dogstatsd metrics, and tags collected by other integrations.


.. _Release Notes_6.5.0_Upgrade Notes:

Upgrade Notes
-------------

- Autodiscovery now enforces the ac_exclude and ac_include filtering options
  for all listeners. Please double-check your exclusion patterns before upgrading
  and add inclusion patterns if some autodiscovered containers match these.

- The introduction of multiple runners for checks implies check
  instances may now run concurrently. This should help the agent
  make better use of resources, in particular it will help prevent
  or reduce the side-effects of slow checks delaying the execution
  of all other checks.

  The change will affect custom checks not enforcing thread safety as
  they may, depending on the schedule, access unsynchronized structures
  concurrently with the corresponding data race ensuing. If you wish to
  run checks in a fully sequential fashion, you may set the `check_runners`
  option in your `datadog.yaml` config or via the `DD_CHECK_RUNNERS` to 1.
  Also, please feel free to reach out to us if you need more information
  or help with the new multiple runner/concurrency model.

  For more details please read the technical note in the `datadog.yaml`_.

  .. _datadog.yaml: https://github.com/DataDog/datadog-agent/blob/main/pkg/config/config_template.yaml#L130-L140

- Prometheus custom checks are now limited to 2000 metrics by default
  to provide users control over the maximum number of custom metrics
  sent in the case of configuration errors or input changes.
  This limit can be changed with the ``max_returned_metrics`` option
  in the check configuration.


.. _Release Notes_6.5.0_Bug Fixes:

Bug Fixes
---------

- All Autodiscovery listeners now enforce the ac_exclude and ac_include filtering
  options, as described in the documentation.

- Fixed "logs_config.frame_size" override that would not be taken into account.

- collect io metrics for drives with path only (like: C:\C0) on Windows

- Fix API_KEY validation for 'additional_endpoints' by using their respective
  endpoint instead of the main one all the time.

- Fix port ordering for the %%port_%% Autodiscovery tag on the docker listener

- Fix missing ECS tags under some conditions

- Change the name of the agent expvar from ``aggregator/ServiceCheckFlushed)``
  to ``aggregator/ServiceCheckFlushed``

- Fix an issue where logs wouldn't be ingested if the API key contains a trailing
  new line

- Setting the log level of the ``check`` subcommand using
  the ``-l`` flag was not setting the log level of python integrations.

- Display embedded Python version in the status page instead of the version
  from the system Python.

- Fixes a bug causing kube_service tags to be missing when kubernetes_map_services_on_ip is false.

- The ntp check now handles negative offsets if the host time is in the
  future.

- Fix a possible index out of range panic in Dogstatsd origin detection

- Fix a verbose debug log caused by rescheduling services with no checks associated with them.


.. _Release Notes_6.5.0_Other Notes:

Other Notes
-----------

- JMXFetch upgraded to 0.20.2; ships updated FasterXML.

- Remove noisy and useless debug log line from contextResolver


.. _Release Notes_6.4.2:

6.4.2
=====

.. _Release Notes_6.4.2_Prelude:

Prelude
-------

Release on: 2018-08-13

- Please refer to the `6.4.2 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-642>`_ for the list of changes on the Core Checks.

.. _Release Notes_6.4.2_Enhancement Notes:

Enhancement Notes
-----------------

- The flare command does not collect the agent container's environment variables anymore


.. _Release Notes_6.4.2_Bug Fixes:

Bug Fixes
---------

- Fixes an issue with docker tailing on restart of monitored containers.
  Previously, at each container restart the agent would re submit all logs.
  Now, on restart we use tracked offsets properly, and as a result submit only
  new logs


.. _Release Notes_6.4.1:

6.4.1
=====

.. _Release Notes_6.4.1_Prelude:

Prelude
-------

Release on: 2018-08-01

- Please refer to the `6.4.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-641>`_ for the list of changes on the Core Checks.

- Please refer to the `6.4.1 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.4.1>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.4.1 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.4.1>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.4.1_New Features:

New Features
------------

- Create packaging for google cloud launcher integration.

- Add options to exclude specific payloads from being sent to Datadog. In
  some environments, some of the gathered information is considered too
  sensitive to be sent to Datadog (i.e. IP addresses in events or service
  checks). This feature adds to option to exclude specific payload types from
  being sent to the backend.

- Collect container disk metrics less often in the docker check, decreasing its effect on performance when enabled.

- Autodiscovery now supports the %%hostname%% tag on the docker listener
  This tag will resolve to the containers' hostname value if present in
  the container inspect. It is useful if the container IP is not available
  or erroneous.

- Dogstatsd origin detection now supports container tagging for Kubernetes clusters
  running containerd or cri-o, in addition to the existing docker support

- This release ships full support of Kubernetes 1.3+

- OpenShift ClusterResourceQuotas metrics are now collected by the kube_apiserver check,
  under the openshift.clusterquota.* and openshift.appliedclusterquota.* names.

- Display the version for Python checks on the status page.


.. _Release Notes_6.4.1_Enhancements Notes:

Enhancement Notes
------------------

- Adding DD_EXPVAR_PORT to the configuration environment variables.

- On Windows, Specifically log to both the log file and the event viewer
  what initiated an agent shutdown.  Also logs specific startup errors
  to both the log file and event viewer.

- The embedded Python has been bumped from 2.7.14 to 2.7.15

- Agent expvar metrics now have default values. Metrics like the number of
  packets dropped by the agent or errors were previously not reported until a
  first event occurred. This should make it easier to use the expvar
  configuration ``agent_stats.yaml``.

- Proxy settings can be configured through the environment variables ``DD_PROXY_HTTP``,
  ``DD_PROXY_HTTPS`` and ``DD_PROXY_NO_PROXY``. These environment variables take precedence over
  the ``proxy`` options configured in ``datadog.yaml``, and behave exactly the same way as these
  options. The standard ``HTTP_PROXY``, ``HTTPS_PROXY`` and ``NO_PROXY`` are still honored but have
  known side effects on integrations, for simplicity we recommended using the new environment variables.
  For more information, please refer to our `proxy docs`_

  .. _proxy docs: https://docs.datadoghq.com/agent/proxy/

- Update to distribution metrics algorithm with improved accuracy

- Added ECS pause containers to the default docker exclusion list

- Adding logging for when the agent fails to detect the origin of a packet in dogstatsd socket mode because of namespace issues.

- The ``skip_ssl_validation`` configuration option can now be set through the related ``DD_SKIP_SSL_VALIDATION`` env var

- The Agent will log failed healthchecks on query and during exit

- On Windows, provides installation parameter to set the `cmd_port`,
  the port on which the agent command interface runs.  To be used if
  the default (5001) is already used by another program.

- The `kube_service` tag is now collected on Kubernetes 1.3.x versions. The matching uses
  a new logic. If it were to fail, reverting to the previous logic is possible by setting
  the kubernetes_map_services_on_ip option to true.

- The Kubernetes event collection timeout is now configurable

- Logs Agent: Added SOCKS5 proxy support. Use ``logs_config: socks5_proxy_address: fqdn.example.com:port`` to set the proxy.

- The diagnose output is now sorted by the diagnosis name

- Adding the status of the DCA (If enabled) in the Agent status command.


.. _Release Notes_6.4.1_Upgrade Notes:

Upgrade Notes
-------------

- If the environment variables that can be used to configure a proxy (``DD_PROXY_HTTP``, ``DD_PROXY_HTTPS``,
  ``DD_PROXY_NO_PROXY``, ``HTTP_PROXY``, ``HTTPS_PROXY`` and ``NO_PROXY``) are present with an empty value
  (e.g. ``HTTP_PROXY=""``), the Agent now uses this empty value instead of ignoring it and using
  lower-precedence options.


.. _Release Notes_6.4.1_Deprecation Notes:

Deprecation Notes
-----------------

- Begin deprecating "Agent start" command.  It is being replaced by "run".  The "start"
  command will continue to function, with a deprecation notice


.. _Release Notes_6.4.1_Security Issues:

Security Issues
---------------

- 'app_key' value from the configuration is now redacted when creating a
  flare with the agent.


.. _Release Notes_6.4.1_Bug Fixes:

Bug Fixes
---------

- Fixes presence of invalid UTF-8 characters when docker log message is greater than 16Kb

- Fix a possible agent crash due to a race condition in the auto discovery.

- Fixed an issue with jmxfetch not being killed on agent exit.

- Errors logged before the agent initialized the log module are now printed
  on STDERR instead of being silenced.

- Detect and handle Docker messages without header.

- Fixes installation, packaging scripts for OpenSUSE LEAP and greater.

- In the event of being unable to lock the `dd-agent` user (eg. `dd-agent`
  is an LDAP user) during installation, do not fail; print relevant warning.

- The leader election process is now restarted if the leader stops leading.

- Avoid Linux package installation failures when both the ``initctl`` and
  ``systemctl`` commands are present but upstart is used as the init system


.. _Release Notes_6.4.1_Other Notes:

Other Notes
-----------

- The system information collected from gohai no longer includes network information
  when the agent is running in a container since the network information is for the
  the container and not the host itself.

- The ntp check now runs every 15 minutes by default to avoid over-loading
  the NTP server pools

- Added new command "run" to the agent.  This command replaces the "start"
  command, to reduce ambiguity with the service lifecycle commands


.. _Release Notes_6.3.3:

6.3.3
=====

.. _Release Notes_6.3.3_Prelude:

Prelude
-------

Release on: 2018-07-17

- Please refer to the `6.3.3 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-633>`_ for the list of changes on the Core Checks.

- Please refer to the `6.3.3 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.3.3>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.3.3 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.3.3>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.3.3_Enhancements Notes:

Enhancements
------------

- Add 'system.mem.buffered' metric on linux system.


.. _Release Notes_6.3.3_Bug Fixes:

Bug Fixes
---------

- Fix the IO check behavior on unix based on 'iostat' tool:

  - Most metrics are an average time, so we don't need to divide again by
    'delta' (ex: number of read/time doing read operations)
  - time is based on the millisecond and not the second

- Kubernetes API Server's polling frequency is now customisable.

- Use as expected the configuration value of kubernetes_metadata_tag_update_freq,
  introduce a kubernetes_apiserver_client_timeout configuration option.

- Fix a bug that led the agent to panic in some cases if
  the ``log_level`` configuration option was set to ``error``.


6.3.2
=====

Prelude
-------

Released on: 2018-07-05

- Please refer to the `6.3.2 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-632>`_ for the list of changes on the Core Checks.


Bug Fixes
---------

- The service mapper now groups the mappings of pods to services by namespace.
  This prevents `kube_service` tags from being erroneously applied to metrics
  for a pod not targeted by a service but has the same name as a pod in a different
  namespace targeted by that service.

- Fix a bug in dogstatsd metrics parsing where the Agent would leave the host tag
  empty instead of applying its hostname on metrics with a tag metadata
  field but no tags (i.e. the tags field is only one `#` character).
  Regression introduced in 6.3.0

- Replace invalid utf-8 characters by the standard replacement char.


6.3.1
=====

Prelude
-------
Release on: 2018-06-27

- Please refer to the `6.3.1 tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-631>`_ for the list of changes on the Core Checks.

- Please refer to the `6.3.1 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.3.1>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.3.1 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.3.1>`_ for the list of changes on the Process Agent.


Upgrade Notes
-------------

- JMXFetch upgraded to 0.20.1; ships tagging bugfixes.


Bug Fixes
---------

- Fixes panic when the agent receives an unsupported pattern in a log processing rule

- Fixes problem in 6.3.0 in which agent wouldn't start on Windows
  Server 2008r2.

- Provide the actual JMX check name as `check_name` in configurations
  provided to JMXFetch via the agent API. This addresses a regression
  in 6.3.0 that broke the `instance:` tag.
  Due to the nature of the regression, and the fix, this will cause
  churn on the tag potentially affecting dashboards and monitors.


.. _Release Notes_6.3.0:

6.3.0
=====

.. _Release Notes_6.3.0_Prelude:

Prelude
-------
Release on: 2018-06-20

- Please refer to the `6.3.0 tag on integrations-core <https://github.com/DataDog/integrations-core/releases/tag/6.3.0>`_
  for the list of changes on the Core Checks.

- Please refer to the `6.3.0 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.3.0>`_
  for the list of changes on the Trace Agent.

- Please refer to the `6.3.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.3.0>`_
  for the list of changes on the Process Agent.


.. _Release Notes_6.3.0_New Features:

New Features
------------

- Add docker memory soft limit metric.

- Added a host tag for docker swarm node role.

- The import command now support multiple dd_url and API keys.

- Add an option to set the read buffer size for dogstatsd socket on POSIX
  system (SO_RCVBUF).

- Add support for port names in template vars for autodiscovery.

- Add a new "tagger-list" command that outputs the tagger content of a running agent.

- Adding Azure pause containers to the default image exclusion list

- Add flag `histogram_copy_to_distribution` to send histogram metric values
  as distributions automatically. Note that the distributions feature is in
  beta. An additional flag `histogram_copy_to_distribution_prefix` modifies
  the existing histogram metric name by adding a prefix, e.g. `dist.`, to
  better distinguish between these values.

- Add docker & swarm information to host metadata

- "[BETA] Encrypted passwords in configurations can now be fetched from a
  secrets manager."

- Add `docker ps -a` output to the flare.

- Introduces a new redacting writer that will make sure anything written into
  the flare is scrubbed from credentials and sensitive information.

- The agent now supports setting/overriding proxy URLs through environment
  variables (HTTP_PROXY, HTTPS_PROXY and NO_PROXY).

- Created a new journald integration to collect logs from systemd. It's only available on debian distributions for now.

- Add kubelet version to container metadata.

- Adds support for windows event logs collection

- Allow overriding procfs path. Should allow to collect relevant host metrics
  in containerized environments. The override will affect python checks and
  will result in psutil using the overriding path.

- The fowarder will now spaw specific workers per domain to avoid slow down when one domain is down.

- ALPHA - Adding new tooling to securely upgrade integration packages/wheels
  from our private TUF repository. Please note any third party dependencies will
  still be downloaded from PyPI with no additional security validation.


.. _Release Notes_6.3.0_Upgrade Notes:

Upgrade Notes
-------------

- If your Agent is configured to use a web proxy through the ``proxy`` config option
  or one of the ``*_PROXY``  environment variables, and the configured proxy URL
  starts with the ``https://`` scheme, the Agent will now attempt to connect to
  your proxy using HTTPS, whereas it would previously connect to your proxy using
  HTTP. If you have a working proxy configuration, please make sure your proxy URL(s)
  start with the  ``http://`` scheme before upgrading to v6.3+. This has no impact on the
  security of the data sent to Datadog, since the payloads are always secured with
  HTTPS between your Agents and Datadog whatever ``proxy`` configuration you may use.

- Docker image: we moved the default configuration from the docker image's default
  environment variables to the `datadog-*.yaml` files. This allows users to easily
  mount a custom `datadog.yaml` configuration file to set all options.
  If you already did so, you will need to update your `datadog.yaml` to include
  these new defaults. If you only used envvars, no change is needed.

- The agent now supports the environment variables "HTTP_PROXY", "HTTPS_PROXY" and
  "NO_PROXY". If set these variables will override the setting in
  datadog.yaml.

- Moves away from the community library for the kubernetes client in favor of the official one.


.. _Release Notes_6.3.0_Deprecations Notes:

Deprecation Notes
-----------------

- The core Agent check Python code is no longer duplicated here and is instead
  pulled from integrations-core. The code now resides in the `datadog_checks`
  namespace, though the old `checks`, `utils`, etc. paths are still supported.
  Please update your custom checks accordingly. For more information, see
  https://github.com/DataDog/datadog-agent/blob/main/docs/agent/changes.md#python-modules


.. _Release Notes_6.3.0_Bug Fixes:

Bug Fixes
---------

- Default config `agent_stats.yaml` used to collect go_expvar metrics from the
  Agent has been updated.

- Take into account empty hosts on metrics coming from dogstatsd, instead of
  ignoring them and applying the Agent's hostname.

- Decrease epsilon and increase incoming buffer size for improved accuracy of
  distribution metrics.

- Better handling of docker return values to avoid errors

- Fix log format when no log file is specified which cause the log date to
  not be correctly displayed.

- Configurations of unscheduled checks are now properly removed from the configcheck command display.

- The agent would send the source twice when protobuf enabled (default),
  once in the source field and once in tags. As a result, we would see the
  source twice in the app. This PR fixes it, by sending it only in the source
  field.

- Fix a bug on windows where the io check was reporting metrics for the ``C:``
  drive only.

- Multiple config files can now be used for the same JMX based integration

- The auto-discovery mechanism can now properly discover multiple configs for one JMX based integration

- The JMXFetch process is now managed properly when JMXFetch configs are unscheduled through auto-discovery

- Fix a possible panic in the kubernetes event watcher.

- Fix panics within the agent when using non thread safe method from Viper
  library (Unmarshall).

- On RHEL/SUSE, stop the Agent properly in the pre-install RPM script on systems where
  ``/lib`` is not a symlink to ``/usr/lib``.

- To match the behavior of Agent 5, a flag has been introduced to make the
  agent use ``hostname -f`` on unix-based systems before trying ``os.Hostname()``.
  This flag is turned off by default for 6.3 and will be enabled by default in 6.4.
  The import command used to upgrade from the Agent5 to the Agent6 will enable
  this flag in the config.

- Align docker agent's kubernetes liveness probe timeout with docker healthcheck (5s) to avoid too many container restarts.

- Fix kube_service tagging of kubernetes network metrics

- Fixed parsing issue with logs processing rules in autodiscovery.

- Prevent logs agent from submitting protocol buffer payloads with invalid UTF-8.

- Fixes JMXFetch on Windows when the ``custom_jar_paths`` and/or ``tools_jar_path`` options are set,
  by using a semicolon as the path separator on Windows.

- Prevent an empty response body from being marked as a "successful call to the GCE metadata api".
  Fixes a bug where hostnames became an empty string when using docker swarm and a non GCE environment.

- Config option specified in `syslog_pem` if syslog logging is enabled with
  TLS should be a path to the certificate, not a textual certificate in the
  configuration.

- Changes the hostname used for Docker events to be the hostname of the agent.

- Removes use of gopsutil on Windows.  Gopsutil relies heavily on WMI;
  because the go runtime doesn't lock goroutines to system threads, the
  COM layer can have difficulties initializing.
  Solves the problem where metadata and various system checks can't
  initialize properly


.. _Release Notes_6.3.0_Other Notes:

Other Notes
-----------

- The agent is now compiled with Go 1.10.2

- The datadog/agent docker image now runs two collector runners by default

- The DEB and RPM packages now create the ``dd-agent`` user with no login shell (``/sbin/nologin``
  or ``/usr/sbin/nologin``). The packages do not modify the login shell of the ``dd-agent`` user
  if it already exists.

- The scripts of the Linux packages now don't exit with errors when no supported init system is detected,
  and only print warnings instead

- On the status and check command outputs, rename checks' ``Metrics`` to ``Metric Samples``
  to reflect that the number represents the number of samples submitted by the check, not
  the number of metrics after aggregation.

- Scrub all logging output from credentials. Should prevent leakage of
  credentials in logs from 3rd-party code or code added in the future.


6.2.1
=====
2018-05-23

Prelude
-------

- Please refer to the `6.2.1 tag on integrations-core <https://github.com/DataDog/integrations-core/releases/tag/6.2.1>`_
  for the list of changes on the Core Checks.

- Please refer to the `6.2.1 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.2.1>`_
  for the list of changes on the Trace Agent.

- Please refer to the `6.2.1 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.2.1>`_
  for the list of changes on the Process Agent.

Known Issues
------------

- If the kubelet is not configured with TLS auth, the agent will fail to communicate with the API when it should still try HTTP.

Bug Fixes
---------

- Fix collection of host tags pulled from GCP project (``project:`` and ``numeric_project_id:`` tags)
  and GCP instance attributes.

- A bug was preventing some jmx configuration options to be set from the jmx
  checks configs.

- The RPM packages now write systemd service files to `/usr/lib/systemd/system/`
  (recommended path on RHEL/SUSE) instead of `/lib/systemd/system/`

6.2.0
=====
2018-05-11

Prelude
-------

- Please refer to the `6.2.0 tag on integrations-core <https://github.com/DataDog/integrations-core/releases/tag/6.2.0>`_
  for the list of changes on the Core Checks.

- Please refer to the `6.2.0 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.2.0>`_
  for the list of changes on the Trace Agent.

- Please refer to the `6.2.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.2.0>`_
  for the list of changes on the Process Agent.

Enhancements
------------

- Introduce new docker cpu shares gauge.

- Add ability to configure the namespace in which the resources related to the kubernetes check are created.

- The kubelet check now honors container filtering options

- Adding Datadog Cluster Agent client in Node Agent.
  Adding support for TLS in the Datadog Cluster Agent API.

- Docker: set a default 5 seconds timeout on all docker requests to mitigate
  possible docker daemon freezes

- Connection to the ECS agent should be more resilient

- Add agent5-like JMXFetch helper commands to help with JMXFetch troubleshooting.

- The agent has been tested on Kubernetes 1.4 & OpenShift 3.4. Refer to
  https://github.com/DataDog/datadog-agent/blob/main/Dockerfiles/agent/README.md
  for installation instructions

- Extract creator tags from kubernetes legacy `created-by` annotation if
  the new `ownerReferences` field is not found

- The `agent import` command now handles converting options from the legacy
  `kubernetes.yaml` file, for agents running on the host

- The memory corecheck sends 2 new metrics on Linux: ``system.mem.commit_limit``
  and ``system.mem.committed_as``

- Added the possibility to filter docker containers by name for log collection.

- Added a support for docker labels to enrich logs metadata.

- Logs Agent: add a `filename` tag to messages with the name of the file being tailed.

- Shipping protobuf C++ implementation for the protobuf package, this should
  help us be more performant when parsing larger/binary protobuf messages in
  relevant integrations.

- Enable to set collect_ec2_tags from environment variable DD_COLLECT_EC2_TAGS

- The configcheck command now display checks in alphabetical orders and are
  no longer grouped by configuration provider

- Add average check run time to ``datadog-agent status`` and to the GUI.

- Consider every configuration having autodiscovery identifier a template

- Implement a circuit breaker and use jittered, truncated exponential backoff for network error retries.

- Change logs agent configuration to use protocol buffers encoding and
  endpoint by default.

Known Issues
------------

- Kubernetes 1.3 & OpenShift 3.3 are currently not fully supported: docker and kubelet
  integrations work OK, but apiserver communication (event collection, `kube_service`
  tagging) is not implemented

Deprecation Notes
-----------------

- Removing python PDH code bundled with the agent in favor of code already included
  in the integrations-core` repository and bundled with datadog_checks_base wheel.
  This provides a single source of truth for the python PDH logic.

Bug Fixes
---------

- Fix a possible race condition in AutoDiscovery where configuration is
  identical on container churn and considered as duplicate before being
  de-scheduled.

- It is now possible to save logs only configuration in the GUI without getting an error message.

- Docker network metrics are now tagged by interface name as a fallback if a
  docker network name cannot be determined (affects some Swarm stack deployments)

- Dogstatsd now support listening on an IPv6 address when using ``bind_host``
  config option.

- The agent now fetches a hostname alias from kubernetes when possible. It fixes some duplicated
  host issues that could happen when metrics were using kubernetes host names, as the
  kubernetes_state integration

- Fix case issues in tag extraction for docker/kubernetes container tags and kubernetes host tags

- Fixes initialization of performance counter (Windows) to be able to better cope with missing
  counter strings, and non-english locales

- Bind the kubelet_tls_verify as an environment variable.

- Docker image: fix entrypoint bug causing the kubernetes_apiserver check
  to not be enabled

- Fixed an issue with collecting logs bigger than 4096 chars on windows.

- Fixes a misleading log line on windows for logs file tailing

- Fixed a concurrent issue in the logs auditor causing the agent to crash.

- Fix an issue for docker image name filtering when images contain a tag.

- On Windows, changes the configuration for Process Agent and Trace
  Agent services to be manual-start.  There is no impact if the
  services are configured to be active; however, if they're disabled,
  will stop the behavior where they're briefly started then stopped,
  which creates excessive Windows service alert.

- API key validation logic was ignoring proxy settings, leading to situations
  where the agent reported that it was "Unable to validate API key" in the GUI.

- Fix EC2 tags collection when multiple marketplaces are set.

- Fixes collection of host tags from GCE metadata

- Fix Go checks errors not being displayed in the status page.

- Sanitize logged Datadog URLs when proxies are configured.

- Fix a race condition in the kubernetes service tagging logic

- Fix a possible panic when docker cannot inspect a container

Other Notes
-----------

- In the metrics aggregator, log readable context information (metric name,
  host, tags) instead of the raw context key to help troubleshooting

- Remove executable permission bits from systemd/upstart/launchd service definition
  files.

- Improved the flare credential removing logic to work in a few edge cases
  that where not accounted for previously.

- Make file tailing a little less verbose. We avoid logging at every iteration
  the different issues we encountered, instead we log them at first run only.
  The status command shows the up-to-date information, and can
  be used at anytime to troubleshoot such issues

- Adds collection of PDH counter information to the flare; saves the
  step of always asking the customer for this information.

- Improve logging for the metamap, avoid spammy error when no cluster level metadata is found.


6.1.4
=====
2018-04-19

Prelude
-------

Our development staff observed that a local, unprivileged user had the ability to make an HTTP request to the `/agent/check-config` endpoint on the agent process that listens on localhost. This request would result in the local-users' ability to read Agent integration configurations. This issue was patched by enforcing authentication via a session token. Please upgrade your agent accordingly.

Security Issues
---------------

- The ``/agent/check-config`` endpoint has been patched to enforce authentication
  of the caller via a bearer session token.


6.1.3
=====
2018-04-16

Prelude
-------

- This release also includes changes to the trace agent. See
  `6.1.3 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.1.3>`_

Bug Fixes
---------

- Fix a bug where the `docker_network` tag incorrectly appeared on
  non-network docker metrics and autodiscovery tags

- Fix the use of "docker restart" with the agent image


6.1.2
==========
2018-04-05

Bug Fixes
---------

- Fix some edge cases where flare could contain secrets if the secrets where encapsulated in quotes.


6.1.1
==========
2018-03-29

Bug Fixes
---------

- Fix a crash in the docker check when collecting sizes on an image with no repository tags.

- Fixes bug on Windows where, if configuration options are specified on the
  installation command line, invalid proxy options are set.

- Removed the read timeout for UDP connections causing the agent to stop forwarding logs after one minute of nonactivity.

- Updating the data type of the CPU of the task and the metadata name for Version to Revision.


Other Notes
-----------

- Add environment variable DD_ENABLE_GOHAI for setting option enable_gohai when running in a container.


6.1.0
=====
2018-03-23

New Features
------------

- Add Agent Version to flare form

- Add the DD_CHECK_RUNNERS environment variable binding

- Add the status command to the DCA.

- Docker check: ignore the new exec_die event type by default

- Extract the swarm_namespace tag for docker swarm containers, in addition
  to the already present swarm_service tag.

- Allow configuration of the enabled-state of process, logs, and apm to be
  specified on the installation command line for Windows.

- Add a jmx_use_cgroup_memory_limit option to set jmxfetch to use cgroup
  memory limits when calculating its heap size. It is enabled by default
  in the docker image.

- Add option to extract kubernetes pod annotations as tags, similar to labels

- Added an environment variable `DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL` to enable logs tailing on all containers.

- Adding the 'bind_host' option to configure the interface to bind by dogstatsd and JMX.

- Support setting tags as a YAML array in the logs agent integration configuration


Bug Fixes
---------

- Fix docker memory metrics parsing from cgroup files

- Fix docker.mem.in_use metric computation

- When using the import script, change the group owner of configuration files to the dd-agent user.

- Fix a false positive in the collector-queue healthcheck

- The old docker_daemon check is now properly converted in the "import" command by default

- Docker check: fix event filtering for exec events

- Improve docker monitoring when the system is under a very high load. The agent
  might still temporarily miss a healthcheck, but will be able to run already
  scheduled checks, and recover once the spike ends

- Fixes the container startup on Fargate, where we tried and remove the same
  file twice, failing hard (stopping) on the second attempt.

- Fix flare failing on zipping individual components

- Fixed an issue where the import script would put an empty histogram aggregates and percentiles in datadog.yaml if they didn't exist in datadog.conf.

- Fix the build for platforms not supporting Gohai.

- Fixes flaw where Windows Performance counters were not properly initialized
  on non EN-US versions of windows

- Menu in system tray reports wrong version (6.0.0) for all versions of Agent.  This fixes the system tray menu to report the correct version.

- Fixing clear passwords in "config-check.log" when sending a flare.

- Allow network proxy settings set on the Windows installation command
  line to be set in the registry, where they'll be translated to the
  configuration

- Accept now short names for docker image in logs configuration file and added to the possibility to filter containers by image name with Kubernetes.

- Fixes an issue that would prevent the agent from stopping when it was tailing logs
  of a container that had no logs.

- fixes an issue with wildcard tailing of logs files on windows

- Allow Linux package uninstallation to proceed without errors even on platforms
  that aren't supported by the Agent

- Fixes agent to run on Server "Core" versions

- Changes default precision of pdh-based counters from int to float. Fixes bug where fidelity of some counters is quite low, especially counters with values between 0 and 1.

- Adds back the removed system.mem.usable metric for Agents running on Windows.

- Avoid multiple initializations of the tagger subsystem


Other Notes
-----------

- Normalize support of nested config options defined with env vars.

- Make the check-rate command more visible when running "check` to get a list of metrics.


6.0.3
=====
2018-03-12

Prelude
-------

- This release also includes bugfixes to the process agent. See diff_.

  .. _diff: https://github.com/DataDog/datadog-process-agent/compare/5.23.1...6.0.3

Bug Fixes
---------

- Fixed the issue preventing from having docker tags when collecting logs from containers.
- Fix docker metrics collection on Moby Linux hosts (default Swarm AMI)


6.0.2
=====
2018-03-07

Critical Issues
---------------

- Packaging issue in 6.0.1 resulted in the release of nightly builds for trace-agent and process-agent. 6.0.2 ships the stable intended versions.


6.0.1
=====
2018-03-07

Enhancements
------------

- Add information about Log Agent checks to the GUI General Status page.


Bug Fixes
---------

- Run the service mapper on all the agents running the apiserver check. Exit before running the rest of the check if the agent is not the leader.

- Fixing docker network metrics collection for the docker check and the process agent on some network configurations.

- Replaces the system.mem.free metric with gopsutil's 'available' and splits the windows and linux memory checks. Previously this reported with a value of 0 and `system.mem.used` was reporting the same as `system.mem.total`

- ".pdh" suffix was added to `system.io` metrics on windows for side-by-side
  testing when changed the collection mechanism, and inadvertently left.

- Fix bug where global tags for PDH based python checks are not read
  correctly from the configuration yaml.

- IE does not support String.prototype.endsWith, add implementation to the
  string prototype to enable the functionality.

- remove `.pdh` suffix from system.io.wkb_s, system.io_w_s, system.io.rkb_s,
  system.io.r_s, system.io.avg_q_sz

- Fix GUI for JMX checks, they are now manageable from the web UI.

- Fix the launch of JMXFetch on windows and make multiplatform treatment of
  the launch more robust.


6.0.0
=====
2018-02-27

Bug Fixes
---------

- Fixes bug in agent hostname command, whereby the configuration library
  wasn't initialized.  This caused `agent hostname` to use the default
  computed hostname, rather than the entry in the configuration file


6.0.0-rc.4
==========
2018-02-23

Enhancements
------------

- Change the kubernetes leader election system to use configmaps instead of endpoints. This
  allows a simpler migration from Agent5, as Agent6 will not require additional permissions.

- Adds in the proc.queue_length and proc.count metrics with the windows version of the Agent.


Bug Fixes
---------

- Process agent service should pass the configuration file argument to the
  executable when launching - otherwise service will always come up on
  reboots.

- Add the windows icon to the Infrastructure List for Agents installed on Windows machines.

- Fix Docker container ``--pid=host`` operations. Previous RCs can cause host system
  instabilities and should not be run in pid host mode.

- Windows: set correct default value for apm config to enabled, so that the trace agent is
  started by default

- Removes deprecated process_agent_enabled flag

- metrics.yaml is not a "configurable" file - it provides default metrics for
  checks and shouldn't be altered. Removed from the GUI configuration file
  list.

- Windows: gopsutil calls to the CPU module require COM threading model to be
  in multi-threaded mode, to guarantee it's safe to make those calls we load
  the python checks setting the right COM concurrency mode first. Once loaded
  we clear the concurrency mode and python checks that might use COM will set
  it as they need.

- Windows: make stop/restart of DatadogAgent service stop/restart dependent
  services accordingly

- Windows: Prevent system tray icon from being displayed more than once

- Windows: Make default start behavior of process-agent consistent with Linux Agent

- Windows: Fix the item launching the web-based GUI in the systray icon menu

- Windows: Process agent service now passes the configuration file argument to the
  executable when launching - otherwise service will always come up on
  reboots.


Other Notes
-----------

- Windows: Added developer documentation regarding the caveats of the COM
  concurrency model and its implications moving forward. The current state affects
  auto-discovery and dynamic scheduling of checks.


6.0.0-rc.3
==========
2018-02-22

Enhancements
------------

- Adds windows systray icon.  System tray icon includes menu options for
  starting/stopping/restarting services, creating a flare, and launching the
  browser-based GUI.

- allow auth token path to be set in the config file

- Implementation for disabling checks from the web UI

- Agent restart message on UI, clears after restart.

- Add SSL support & label joins for the prometheus check


Bug Fixes
---------

- Fix the command-line flag parsing regression caused by a transitive dependency importing the
  glog library. ``agent`` flags should now behave as in beta9.

- GUI had broken after the introduction of integrations as wheels this PR
  ensures we collect the full list of available integrations so we can
  enable the corresponding configurations from the UI.

- Fix an issue preventing logs-agent to tail container logs when docker API version is prior to 1.25

- Fix line miss issue_ that could happen when tailing new files found when scanning

  .. _issue: https://github.com/DataDog/datadog-agent/issues/1302

- On windows ``Automatic`` services would fail to start across reboots due to
  a known go issue on 1.9.2: https://github.com/golang/go/issues/23479
  We now start windows services as delayed start automatic services (ie. they
  now start automatically after all other automatic services).


Other Notes
-----------

- The OSX build of the agent does not include the containers integrations
  as they are only supported on Linux for now. The Windows build already
  excluded them since beta1

- The ``auth_token`` file, used to store the api authentication token, is now
  only readable/writable by the user running the agent instead of inheriting
  datadog.yaml permissions.


6.0.0-rc.2
==========
2018-02-20

New Features
------------

- Add namespace configuration for metric names for dogstatsd

- Rework autodiscovery label names to be consistent, still support the
  previous names

- Ships updated integrations from `integrations-core 6.0.0-rc.2`_, including new ``kubelet`` check

  .. _integrations-core 6.0.0-rc.2: https://github.com/DataDog/integrations-core/releases/tag/6.0.0-rc.2

- Add envvar bindings for docker/kubernetes custom tag extraction features


Upgrade Notes
-------------

- Normal installations: APM now listens to localhost only by default, you need to set
  `apm_config.apm_non_local_traffic = true` to enable listening on the network

- Docker image: APM is now disabled by default, you need to set `DD_APM_ENABLED=true`
  to run the trace agent. It listens on all interfaces by default when running, you can
  set `DD_APM_NON_LOCAL_TRAFFIC=false` to only listen on localhost


Bug Fixes
---------

- Don't try to match containers by image name if they provide an AD template
  via docker labels or pod annotations. This avoid scheduling double checks.

- Fix handling of the %%host%% and %%port%% autodiscovery tags

- The aggregator now discards metric samples with ``NaN`` values. Also solves a serializing error
  on metric payloads.

- Fixes bug whereby device tag was (correctly) removed from tags list, but
  device field was only added to the metric on the first run.

- Fix an issue unscheduling checks discovered through auto-discovery

- Upstart would indefinitely respawn trace and process agents even when exiting
  with a zero status code. We now explicitly define exit code 0 as a valid exit
  code to prevent respawn when the agents are disabled.

- Fix cases where empty host tags in the Agent ``datadog.yaml`` configuration caused
  the host metadata payload parsing to fail in the backend.

- Fix ``resources`` metadata collector so that its payload is correctly parsed in the
  backend even when empty.

- Make sure we don't get stuck if the API server does not return events.

- make tagger more resilient to malformed docker events

- Removing `vsphere` and `sqlserver` from the blacklist. The former is
  available on all platforms, `sqlserver` is currently windows-only.


Other Notes
-----------

- The `apm.yaml.default` config file was removed on linux and the
  `trace-agent.conf.example` was removed on every other platform.

- Only enable the ``resources`` metadata collector on Linux by default, to match
  Agent 5's behavior.


6.0.0-rc.1
==========
2018-02-16

Prelude
-------

The execution of the main agent, trace-agent (APM), and process-agent processes is now orchestrated
using systemd/upstart facilities on Linux.

On Linux and macOS, the trace-agent and process-agent now read their configuration from the main
``datadog.yaml`` file (located, by default on Linux, at ``/etc/datadog-agent/datadog.yaml``).

Changes implementation of IOStats in Windows to use the Performance Data  Helper API, rather than WMI.


New Features
------------

- Introducing the Datadog Process Agent for Windows

- Make the trace-agent read its configuration from datadog.yaml.

- Add a HTTP header containing the agent version to every transaction sent by
  the agent.

- Add a configcheck command to the agent API and CLI, it prints current loaded & resolved
  configurations for each provider. The output of the command is also added to a new config-check.log
  into the flare.

- Add option to output logs in JSON format

- Add a pod entity to the tagger.

- Introducing the service mapper strategy for the DCA. We periodically hit the API server,
  to get the list of pods, nodes and services. Then we proceed to match which endpoint (i.e. pod)
  is covered by which service.
  We create a map of pod name to service names, cache it and expose a public method.
  This method is called when the Service Mapper endpoint of the DCA API is hit.
  We also query the cache instead of the API Server if a cache miss happens, to separate the concerns.

- The kubelet request /pods is now cached to avoid stressing the kubelet API and
  for better performances

- Create a cluster agent util to query the Datadog Cluster Agent (DCA) API.

- Convert the format of histogram_aggregate when importing agent5 configs to agent6

- Add support for histogram_percentile when importing config options from agent5 to agent6

- Adds in a `datadog.agent.running` metric that showcases a value of 1 if the Agent is currently reporting to Datadog.

- Use the S6 light init in the docker image to start the process-agent and the
  trace-agent. This allows to remove the process orchestration logic from the
  infra-agent

- Add `short_image` tag to docker tagger collector

- Try to connect to Docker even `/var/run/docker.sock` is absent, to honor the
  $DOCKER_HOST environment variable.

- Get the docker server version in the debug logs for troubleshooting

- Add ECS & ECS Fargate metadata API connectivity diagnose

- Add optional forwarding of dogstatsd packets to another UDP server.

- Don't block the aggregator if the forwarder input queue is full (we now
  drop transaction).

- Add healthcheck to the forwarder. The agent will be unhealthy if the apikey
  is invalid or if transactions are dropped because all workers are busy.

- Make the number of workers used by the forwarder configurable and set default to 1.

- The agent has internal healthchecks on all subsystems. The result is exposed
  via the `agent health` command and used in the docker image as a healthcheck.
  The `/probe.sh` wrapper is provided for compatibility with agent5 and future-proofing.

- Adds the ability to import the Trace configuration options from datadog.conf to their respective datadog.yaml fields with the `import` command.

- Add jmx_custom_jars option to make sure they are loaded by jmxfetch

- Added ability to retrieve the hostname from the Kubernetes kubelet API.

- Support the new `kubelet` integration by providing the kubelet url and credentials to it

- Add source_component and namespace tags to Kubernetes events.

- Support the Kubernetes service name tags on containers.
  The tag used is kube_service.
  The default listener for kubernetes is now the kubelet instead of docker.

- Implementation of the Leader Election for the node agent.

- Add more details to the information displayed by the logs agent status.

- Log lines starting with the special character `<` were considered rfc5424
  formatted, and any further formatting was skipped. This commit updates the
  detection rule to match logs starting with `<pri>version `, to reduce
  false positives

- Support tagging on Nomad 0.6.0+ clusters

- adds a core check and the core logic to query the kubernetes API server and format the events into Datadog events. Also adds the Control Plane status check.

- The process-agent uses the datadog.yaml file for activation and additional config.

- Support the new `kubelet` integration by providing the container tags to it

- Set configurable timeout for IPC api server

- Add support for collect_ec2_tags configuration option.

- Support the legacy default_integration_http_timeout configuration option.

- handle the "warning" value to log_level, translating it to "warn"

- Use systemd/upstart facilities on linux to orchestrate the agents execution.
  Rate limit process restarts such that 5 failures in a 10 second span will
  result in no further restart attempts.


Upgrade Notes
-------------

- Increased the number of versions of Docker API that logs-agent support from 1.25 to 1.18

- If you run a Nomad agent older than 0.6.0, the `nomad_group`
  tag will be absent until you upgrade your orchestrator.


Deprecation Notes
-----------------

- Changed the attribute name to enable log collection from YAML configuration file from "log_enabled" to "logs_enabled", "log_enabled" is still supported.


Bug Fixes
---------

- Properly listen for events emitted on OSes like CentOS 6 so the Agent starts on reboot

- Relieves CPU consumption of the WMI service by using PDH rather than WMI

- Updating custom error used to insure the collection of the token in the configmap datadogtoken.

- Fix docker event reconnection logic to gracefully wait if docker daemon is unresponsive

- The checks packaged with the new wheels method are in the default python
  package site, already included in ``sys.path``. We therefore removed this
  path them from the locations that are appended to the default python ``sys.path``

- Strip hostname if running inside ECS Fargate and disable core checks (not relevant
  without host infos)
  Fix hostname caching consistency

- The agent/dogstatsd docker image now ships the appropriate binary

- Fix the uploaded file name of the flare archive.

- Fixed the structure of the Flare archive on all platforms.

- Fix the import proxy build conversion

- Fix the agent stop command

- Fixes a bug that caused the GUI to create a flare without a status file

- Fix https://github.com/DataDog/datadog-agent/issues/1159 where error was not
  explicit when the check had invalid configuration or code

- Fix to evaluate whether the DCA can query resources (events, services, nodes, pods) before running core component or checks.
  Logic allows for the DCA to run components independently if they are configured and we can query the associated resources.

- Only load and log the kube_apiserver check if `KUBERNETES=yes` is used.

- The collector will no longer block when the number of runners is lower than
  the number of long running check. We now start a new runner for those
  checks.

- Modify JMXFetch jar permissions to allow the agent to read it on osx.

- Deleted linux-specific configuration files on macosx to avoid polluting the logs and the web ui.

- Fix dogstatsd unix socket rights so every user could write to it.


Other Notes
-----------

- The ``resources`` metadata provider is now enabled by default in order to
  populate the "process treemap" visualization continuously (on host dashboards)

- Decrease verbosity of ``urllib3``'s logger (used by python checks through the ``requests`` module)

- Document the exclusion of dockercloud containers.

- The flare now includes a dump of whitelisted environment variables
  that can have an impact on agent behaviour. If no whitelisted envvar
  is found, the envvars.log file is not created in the flare.

- Upgraded Go runtime to `1.9.4` on Linux builds

- The linux packages now own the custom check directory (``/etc/datadog-agent/checks.d/``)
  and the log directory (``/var/log/datadog/``)

- Add an automated install script for osx.


6.0.0-beta.9
============
2018-01-26

Prelude
-------

In this release, the order of precedence of the Custom Checks has changed. This
may affect your custom checks. Please refer to the Upgrade Notes section
for more details.

This release includes support for Datadog Logs for Windows.


New Features
------------

- In this release, the Datadog Log feature is supported on all supported
  versions of windows.

- Support AD on Rancher 1.x by getting the container port through the
  image's exposed ports as a fallback mechanism

- APM / Process / Log agents can be enabled/disabled via the DD_*_ENABLED
  environment variables, see the agent docker image readme for details

- Add a dd-agent user in the docker image to prepare for running root-less

- Config parsing errors are now displayed in the output of the 'status' command and on the web ui.

- The DD_TAGS environment variable allows to set host tags, in addition
  to the `tags` option in datadog.yaml

- Added a section to the agent status to report live information about logs-agent

- Set the default "procfs_path" configuration to `/host/proc` when containerized
  to allow the network check to collect the host's network metrics. This can be
  overridden with the the DD_PROCFS_PATH envvar.

- Series for a common metric name will no longer be split among multiple
  transactions/payload. This guarantee that every point for a time T and a metric
  M will be bundled together when push to the backend. This allows some
  optimization on the backend side.

- Add a service listener for ECS Fargate, and a config provider. Also add
  the concept of ECSContainer and make it compatible with Docker containers
  so that the process agent can handle them.


Known Issues
------------

- Having a separate type of container for ECS is not ideal, we will need to
  change how we represent containers to make it more generic. We also need
  to improve hostname handling, this will come in a follow up PR.


Upgrade Notes
-------------

- Custom checks (located by default on Linux in ``/etc/datadog-agent/checks.d/``) now have a
  *lower* precedence than the checks that are bundled with the Agent. This means that a custom
  check with the same name as a bundled check will now be ignored, and the bundled check will be
  loaded instead. If you want to override a bundled check with a custom check, please use a
  new name for your custom check, and use that new name for the related yaml configuration file.

- Tags in the DD_TAGS environment variable are now separated by spaces
  instead of commas in agent5


Bug Fixes
---------

- Added a support for multi-line tailing with docker

- Fix the extraction of the environment variables in the pkg/tagger/collectors/docker_extract.go when the variable is
  like "KEY="

- Fix a nil-pointer segfault in docker event processing when an event is ignored

- Make yum revalidate the cache before installing the rpm package when using
  the install script.

- Added some missing spaces in logs

- Fixed build pipeline commenting flaky tests from logs tailer

- Fixed a bug on file tailing causing the logs-agent to reprocess multiple times the same data when restarted because of a wrong file offset management when lines are trimmed.

- Be more lenient when filtering containers using labels.

- Fixes an issue when pulling tags from a yaml config file with any integration
  that uses the PDHBaseCheck class


Other Notes
-----------

- Updated the shipped CA certs to latest

- For CircleCI use builder images in the `datadog` dockerhub repo.


6.0.0-beta.8
============
2018-01-11

New Features
------------

- Logs-agent now runs as a goroutine in the main agent process

- All docker event subscribers are multiplexed in one connection, reduces stress on the docker daemon

- The Agent can find relevant listeners on its host by using the "auto" listener, for now only docker is supported

- The Docker label AD provider now watches container events and
  only updates when containers start/die to save resources

- Add a new option, `force_tls_12`, to the agent configuration to force the
  TLS version to 1.2 when contactin Datatog.

- Reno and releasenotes are now mandatory. A test will fail if no
  releasenotes where added/updated to the PR. A 'noreno' label can be added
  to the PR to skip this test.


Bug Fixes
---------

- [logs] Fix an issue when the hostname was not provided in datadog.yaml: the logs-agent logic uses the same hostname as the main agent

- [logs] Trim spaces from single lines

- Fix missing fields in forwarder logging entries

- Fix RancherOS cgroup mountpoint detection

- [linux packaging] Fix missing dd-agent script after upgrade, the fix will take effect on a fresh install of '>= beta.8' or upgrade from '>= beta.8'

- [logs] Do not send empty logs with multilines

- [flare] Fix command on Windows by fixing path of collected log files

- Fix path of logs collected by flare on Windows, was breaking flare command


Other Notes
-----------

- Remove resversion handling from podwatcher, as it's unused

- Refactor corecheck boilerplate in CheckBase

- [flare] Rename config file dumped from memory
