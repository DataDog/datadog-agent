=============
Release Notes
=============

.. _Release Notes_6.4.0:

6.4.0
=====

.. _Release Notes_6.4.0_Prelude:

Prelude
-------

Defer process orchestration to the operating system where supported.


Release on: 2018-07-31

- Please refer to the `6.4.0 tag on integrations-core <https://github.com/DataDog/integrations-core/releases/tag/6.4.0>`_ for the list of changes on the Core Checks.

- Please refer to the `6.4.0 tag on trace-agent <https://github.com/DataDog/datadog-trace-agent/releases/tag/6.4.0>`_ for the list of changes on the Trace Agent.

- Please refer to the `6.4.0 tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/6.4.0>`_ for the list of changes on the Process Agent.


.. _Release Notes_6.4.0_New Features:

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

- Adds support for OS-level process orchestration of the agents on linux.
  Specifically, this adds support for upstart on systemd which should cover
  most modern debian/rhel based distros. Changes enable a more resilient 
  process management for the infrastructure, process and trace agents.

- Display the version for Python checks on the status page.


.. _Release Notes_6.4.0_Enhancements Notes:

Enhancements Notes
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


.. _Release Notes_6.4.0_Upgrade Notes:

Upgrade Notes
-------------

- If the environment variables that can be used to configure a proxy (``DD_PROXY_HTTP``, ``DD_PROXY_HTTPS``,
  ``DD_PROXY_NO_PROXY``, ``HTTP_PROXY``, ``HTTPS_PROXY`` and ``NO_PROXY``) are present with an empty value
  (e.g. ``HTTP_PROXY=""``), the Agent now uses this empty value instead of ignoring it and using
  lower-precedence options.


.. _Release Notes_6.4.0_Deprecation Notes:

Deprecation Notes
-----------------

- Begin deprecating "Agent start" command.  It is being replaced by "run".  The "start"
  command will continue to function, with a deprecation notice


.. _Release Notes_6.4.0_Security Issues:

Security Issues
---------------

- 'app_key' value from the configuration is now redacted when creating a
  flare with the agent.


.. _Release Notes_6.4.0_Bug Fixes:

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


.. _Release Notes_6.4.0_Other Notes:

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

- Please refer to the `6.3.3 tag on integrations-core <https://github.com/DataDog/integrations-core/releases/tag/6.3.3>`_ for the list of changes on the Core Checks.

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

- Please refer to the `6.3.2 tag on integrations-core <https://github.com/DataDog/integrations-core/releases/tag/6.3.2>`_ for the list of changes on the Core Checks.


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

- Please refer to the `6.3.1 tag on integrations-core <https://github.com/DataDog/integrations-core/releases/tag/6.3.1>`_ for the list of changes on the Core Checks.

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
  https://github.com/DataDog/datadog-agent/blob/master/docs/agent/changes.md#python-modules


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

- Prevent an empty response body from being marked as a "successfull call to the GCE metadata api".
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
  https://github.com/DataDog/datadog-agent/blob/master/Dockerfiles/agent/README.md
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

- Accept now short names for docker image in logs configuration file and added to the possibilty to filter containers by image name with Kubernetes.

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
  overriden with the the DD_PROCFS_PATH envvar.

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

