=============
Release Notes
=============

6.0.0-rc.2
==========

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

