=============
Release Notes
=============

.. _Release Notes_7.20.0:

7.20.0 / 6.20.0
======

.. _Release Notes_7.20.0_Prelude:

Prelude
-------

Release on: 2020-06-09

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

  .. _datadog.yaml: https://github.com/DataDog/datadog-agent/blob/master/pkg/config/config_template.yaml#L130-L140

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
