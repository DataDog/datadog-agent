# StackState Agent v2 releases

## 2.11.0 (xxxx-xx-xx)

**Improvements**
- Integrations
  - Added support to override the check state location in the configuration of a check. [(STAC-12220)](https://stackstate.atlassian.net/browse/STAC-12220)
  - Added support to map user defined stackstate-environment tags or config to the environments object [(STAC-11132)](https://stackstate.atlassian.net/browse/STAC-11132)
  - Added support to map user defined stackstate-layer tags or config to the layer object [(STAC-11132)](https://stackstate.atlassian.net/browse/STAC-11132)
  - Added support to map user defined stackstate-domain tags or config to the domain object [(STAC-11132)](https://stackstate.atlassian.net/browse/STAC-11132)
  - Added support to map user defined stackstate-identifiers tags or config to the identifiers array [(STAC-11132)](https://stackstate.atlassian.net/browse/STAC-11132)
  - Remove `stackstate-identifier`, `stackstate-environment`, `stackstate-layer`, `stackstate-domain` and `stackstate-identifiers` from the tags object if it has been mapped to the data object. [(STAC-11132)](https://stackstate.atlassian.net/browse/STAC-11132)
  - Improved out-of-the-box support for Kubernetes 1.18+ by automatically falling back to using TLS without verifying CA when communicating with the secure Kubelet [(STAC-12205)](https://stackstate.atlassian.net/browse/STAC-12205)

**Bugfix**

  - Kubelet check should not fail for Kubernetes 1.18+ (due to deprecated `/spec` API endpoint) [(STAC-12307)](https://stackstate.atlassian.net/browse/STAC-12307)

## 2.10.0 (2021-02-25)

**Features**

- Docker Integration
  - The Docker integration is enabled by default for linux and dockerized installations which will produce docker-specific telemetry. [(STAC-11903)](https://stackstate.atlassian.net/browse/STAC-11903)
    - StackState will create a DEVIATING health state for spurious restarts on a container.
- Disk Integration
  - The Disk integration is enabled by default which will produce topology and telemetry related to disk usage of the agent host. [(STAC-11902)](https://stackstate.atlassian.net/browse/STAC-11902)
    - StackState will create a DEVIATING health state on a host when disk space reaches 80% and CRITICAL at 100%.

**Improvements**

- Integrations:
  - Added support to configure Process Agent using `sts_url` [(STAC-11215)](https://stackstate.atlassian.net/browse/STAC-11215)
  - Provide default url for install script [(STAC-11215)](https://stackstate.atlassian.net/browse/STAC-11215)
- Nagios Integration:
  - Added event stream for passive service state events [(STAC-11119)](https://stackstate.atlassian.net/browse/STAC-11119)
  - Added event stream for service notification events [(STAC-11119)](https://stackstate.atlassian.net/browse/STAC-11119)
  - Added event stream for service flapping events [(STAC-11119)](https://stackstate.atlassian.net/browse/STAC-11119)
  - Added event stream check for host flapping alerts [(STAC-11119)](https://stackstate.atlassian.net/browse/STAC-11119)
- vSphere:
  - Topology and properties collection [(STAC-11133)](https://stackstate.atlassian.net/browse/STAC-11133)
  - Events collection [(STAC-11133)](https://stackstate.atlassian.net/browse/STAC-11133)
  - Metrics collection [(STAC-11133)](https://stackstate.atlassian.net/browse/STAC-11133)
- Zabbix:
  - Replace `yaml.safe_load` with `json.loads` [(STAC-11470)](https://stackstate.atlassian.net/browse/STAC-11470)
  - Move stop snapshot from finally block and use StackPackInstance [(STAC-11470)](https://stackstate.atlassian.net/browse/STAC-11470)
  - Send OK Service Check if successful [(STAC-11470)](https://stackstate.atlassian.net/browse/STAC-11470)
- Kubernetes Integration
  - Show Kubernetes secret resources as components in StackState [(STAC-12034)](https://stackstate.atlassian.net/browse/STAC-12034)
  - Show Kubernetes namespaces as components in StackState [(STAC-11382)](https://stackstate.atlassian.net/browse/STAC-11382)
  - Show ExternalName of Kubernetes services as components in StackState [(STAC-11523)](https://stackstate.atlassian.net/browse/STAC-11523)

**Bugfix**

- Integrations:
  - Agent Integrations are not tagged with Check instance tags [(STAC-11453)](https://stackstate.atlassian.net/browse/STAC-11453)
  - Don't create Job - Pod relations from Pods that finished running [(STAC-11490)](https://stackstate.atlassian.net/browse/STAC-11521)
  - Process Agent restart bug fixed for older kernel versions
- Nagios:
  - Shows correct check name in Event details [(STAC-11119)](https://stackstate.atlassian.net/browse/STAC-11119)


## 2.9.0 (2020-12-18)

**Features**

- DynaTrace Topology Integration:
  - Create the topology in StackState from Dynatrace smartscape topology [(STAC-10499)](https://stackstate.atlassian.net/browse/STAC-10499)
- Added support for integrations to send events that can be linked to topology in StackState using Event Context [(STAC-10660)](https://stackstate.atlassian.net/browse/STAC-10660)
- ServiceNow Integration:
  - ServiceNow Change Request are monitored in StackState with all updates to the Change Request state reflected as external events in StackState, such that potential failures can be related to a change in ServiceNow [(STAC-10665)](https://stackstate.atlassian.net/browse/STAC-10665)
  - Added support for filtering ServiceNow CI's using a custom `sysparm_query` [(STAC-11357)](https://stackstate.atlassian.net/browse/STAC-11357)
  - Support for custom cmdb_ci field that acts as Configuration Item identifier [(STAC-11517)](https://stackstate.atlassian.net/browse/STAC-11517)
- Integrations:
  - Added local persistent state that can be used by integrations to persist a JSON object per check instance to disk [(STAC-11296)](https://stackstate.atlassian.net/browse/STAC-11296)
  - Support dynamic identifier building from check configuration using `identifier_mappings` [(STAC-11144)](https://stackstate.atlassian.net/browse/STAC-11144)
- Kubernetes Integration:
  - Map Kubernetes events to Kubernetes components in StackState as events [(STAC-11322)](https://stackstate.atlassian.net/browse/STAC-11322)
  - Add extra topology component for ExternalName K8s services, that can merge with the actual service in use [(STAC-11523)](https://stackstate.atlassian.net/browse/STAC-11523)
  - Add namespace as components [(STAC-11326)](https://stackstate.atlassian.net/browse/STAC-11326) and create relations to Agent for Kubernetes different resource types(Stateful, DaemonSet and etc) [(STAC-11387)](https://stackstate.atlassian.net/browse/STAC-11387)

**Improvements**

- ServiceNow Integration:
  - Added support for batch queries. This can be set with new parameter `batch_size` in check configuration file [(STAC-10855)](https://stackstate.atlassian.net/browse/STAC-10855)
- Integrations:
  - Kubernetes, Kubelet, Kubernetes State and OpenMetrics integrations are monitored by StackState [(STAC-11453)](https://stackstate.atlassian.net/browse/STAC-11453)
  - Check API supports auto snapshots when setting `with_snapshots` to True in the TopologyInstance [(STAC-10885)](https://stackstate.atlassian.net/browse/STAC-10885)
  - Sanitize events and topology data in the base check, encoding unicode to string, before propagating data upstream [(STAC-11298)](https://stackstate.atlassian.net/browse/STAC-11298)
  - Added functionality to the Identifiers utility to provide lower-cased identifiers for all StackState-related identifiers [(STAC-11541)](https://stackstate.atlassian.net/browse/STAC-11541)
- Trace agent:
  - Interpret Traefik traces so that the Traefik component is not the parent of a service-instance [(STAC-10847)](https://stackstate.atlassian.net/browse/STAC-10847)

**Bugfix**

- VSphere Integration:
  - Reconnect on an authentication session timeout [(STAC-11097)](https://stackstate.atlassian.net/browse/STAC-11097)
  - Metric collection for components in Vsphere are now limited to the configured `config.vpxd.stats.maxQueryMetrics` value [(STAC-11313)](https://stackstate.atlassian.net/browse/STAC-11313)
- Integrations:
  - Fixed python2 utf-8 string encoding in data produced by all integrations [(STAC-11294)](https://stackstate.atlassian.net/browse/STAC-11294)
  - Fixed spurious updates of Agent Integrations components in StackState [(STAC-11453)](https://stackstate.atlassian.net/browse/STAC-11453)
  - Fixed a memory leak in the integrations caused by `yaml.safe_load` when loading large objects [(STAC-11363)](https://stackstate.atlassian.net/browse/STAC-11363)
- Nagios Integration:
  - Fixes service name not visible in check details [(STAC-11119)](https://stackstate.atlassian.net/browse/STAC-11119)
- Remove reference of `datadog` in the log for core `ntp` check [(STAC-11017)](https://stackstate.atlassian.net/browse/STAC-11017)
- Network Tracer:
  - Fix loopback address detection [(STAC-8617)](https://stackstate.atlassian.net/browse/STAC-8617)
  - Treat inability to start network tracing as a breaking error [(STAC-11445)](https://stackstate.atlassian.net/browse/STAC-11445)
- Kubernetes Integration:
  - Make HostPath volumes be treated as volumes rather than persistent volumes [(STAC-11515)](https://stackstate.atlassian.net/browse/STAC-11515)

## 2.8.0 (2020-09-27)

**Features**

- Nagios integration: adds support for Nagios ITRS OP5 [(STAC-8598)](https://stackstate.atlassian.net/browse/STAC-8598)
- SAP integration: support tags, domain and environment coming from instance config [(STAC-10659)](https://stackstate.atlassian.net/browse/STAC-10659)
- Zabbix integration: support for maintenance mode [(STAC-10430)](https://stackstate.atlassian.net/browse/STAC-10430)
- SAP integration: Simplify and remove querying SAPHostAgent (Dennis Loos - CTAC)
- Agent integrations [(STAC-9816)](https://stackstate.atlassian.net/browse/STAC-9816)
  - Adds the AgentIntegrationInstance which is a type of TopologyInstance that is synchronized by the Agent StackPack.
  - Allows mapping streams and health checks onto Agent Integration components.
  - Publish Agent Integration components for all running integrations in an agent on which the service checks produced by the integration is mapped and monitored.
  - Added the utility function that allow you to create identifiers in the format that is used in StackState for merging topology.
- Ensure that cluster name tag is present when running on kubernetes [(STAC-10046)](https://stackstate.atlassian.net/browse/STAC-10046)

**Bugfix**

- Nagios integration: adds missing data to events generated from Nagios log [(STAC-10614)](https://stackstate.atlassian.net/browse/STAC-10614)

## 2.7.0 (2020-07-27)

**Features**

- Adds OpenMetrics integration [(STAC-9940)](https://stackstate.atlassian.net/browse/STAC-9940)
- ServiceNow reports and filters certain resource types and relations on the basis of configuration defined. Identifiers added for merging with other integrations. ServiceNow Integration reports all resource types by default. [(STAC-9512)](https://stackstate.atlassian.net/browse/STAC-9512)
- Migrated Nagios Integration to Agent V2. Nagios check gathers topology and metrics from your Nagios instance. [(STAC-8556)](https://stackstate.atlassian.net/browse/STAC-8556)

**Bugfix**

- vSphere integration should continue even if metadata is not present or throws an exception. [(STAC-9373)](https://stackstate.atlassian.net/browse/STAC-9373)

## 2.6.0 (2020-07-02)

**Features**
- ServiceNow check add which provides support to visualize the Configuration Items from your ServiceNow instance. [(STAC-8557)](https://stackstate.atlassian.net/browse/STAC-8557)

**Improvements**

- Short-lived processes (by default, observed for fewer than 60sec) are filtered and not reported to StackState. [(STAC-6356)](https://stackstate.atlassian.net/browse/STAC-6356)
- Network connections made by filtered processes (short-lived / blacklisted) are filtered and not reported to StackState. [(STAC-6286)](https://stackstate.atlassian.net/browse/STAC-6286)
- Short-lived network relations (network connections that are not reported more than once within a configured time window) are filtered and not reported to StackState. [(STAC-9182)](https://stackstate.atlassian.net/browse/STAC-9182)

**Bug Fixes**
- IP based Identifiers for pods are prefixed with the namespace and pod name if HostIP is used on Kubernetes. [(STAC-9451)](https://stackstate.atlassian.net/browse/STAC-9451)
- Added kubernetes namespace to external ID's for all Kubernetes topology components. [(STAC-9375)](https://stackstate.atlassian.net/browse/STAC-9375)
- Fix the data type for extra metadata collection in VSphere integration. [(STAC-9329)](https://stackstate.atlassian.net/browse/STAC-9329)

## 2.5.1 (2020-05-10)

**Improvements**

- Added configuration flag to skip hostname validation [(STAC-7652)](https://stackstate.atlassian.net/browse/STAC-7652).

## 2.5.0 (2020-04-30)

**Features**

- Interpret Spans for topology creation [(STAC-4879)](https://stackstate.atlassian.net/browse/STAC-4879).

**Bugs**

- Fix JMX metric collection [(STAC-5254)](https://stackstate.atlassian.net/browse/STAC-5254)

## 2.4.0 (2020-04-23)

**Features**

- StaticTopology check [(STAC-8524)](https://stackstate.atlassian.net/browse/STAC-8524) provides support to visualize the topology ingested through CSV files.
    * Gathers Topology from CSV files and allows visualization of your ingested components and relations.

- Enable Client Certificate Authentication for SAP integration check [(STAC-8396)](https://stackstate.atlassian.net/browse/STAC-8396).


## 2.3.1 (2020-04-03)

**Bugs**

- Fix VSphere Check functionality [(STAC-8351)](https://stackstate.atlassian.net/browse/STAC-8351)

## 2.3.0 (2020-03-26)

**Features**

- Zabbix check [(STAC-7601)](https://stackstate.atlassian.net/browse/STAC-7601) provides support to visualize the hosts systems monitored by Zabbix.
    * Gathers Topology from your Zabbix instance and allows visualization of your monitored systems components.
    * Provides events mapped to those monitored systems from Zabbix.
    * Disabling a trigger should clear health state [_(STAC-8176)_](https://stackstate.atlassian.net/browse/STAC-8176).
    * Acknowledging a problem in Zabbix should clear state [_(STAC-8177)_](https://stackstate.atlassian.net/browse/STAC-8177) .

**Bugs**

- Trace-agent logs can be found in `C:\ProgramData\StackState\logs` now. [_(STAC-8177)_](https://stackstate.atlassian.net/browse/STAC-8177)

## 2.2.1 (2020-03-18)

**Bugs**

- Fix out of memory issue for vsphere check due to unicode data in topology [(STAC-8113)](https://stackstate.atlassian.net/browse/STAC-8113)

## 2.2.0 (2020-03-09)

**Features**

- SCOM check [(STAC-7551)](https://stackstate.atlassian.net/browse/STAC-7551) provides support to visualize the systems monitored by SCOM.
    * Gathers Topology from your SCOM management pack and allows visualization of your monitored systems components and the relations between them.
    * Monitoring of your SCOM (as well as systems monitored  by SCOM), including health statuses of all your components.

- Vsphere Check [(STAC-7516)](https://stackstate.atlassian.net/browse/STAC-7516) used to create a near real time synchronization with VMWare VSphere VCenter.
    * Gathers Topology from your Vsphere instance and allows visualization of your monitored systems components and the relations between them.

**Improvements**

- Metrics produced by the Kubernetes Agent Checks now produce a cluster name tag as part of the metric. [(STAC-8095)](https://stackstate.atlassian.net/browse/STAC-8095)

## 2.1.0 (2020-02-11)

**Features**

- AWS X-Ray check [(STAC-6347)](https://stackstate.atlassian.net/browse/STAC-6347)
    * This check provides real time gathering of AWS X-Ray traces that allows mapping the relations between X-Ray services, and ultimately AWS resources provided from AWS StackPack.
    * It provides performance metrics, as well as local anomaly detection on all performance metrics based on AWS X-Ray traces

- SAP check [(STAC-7515)](https://stackstate.atlassian.net/browse/STAC-7515)
    * This check provide host instance metrics:
        + available memory metric
        + DIA free worker processes
        + BTC free worker processes

    * Ensure SAP host instances merge with vsphere VMs
    * Add `stackpack:sap` label to the StackPack

**Improvements**

- Added kubernetes cluster name, namespace and pod name as a tag to all kubernetes container and process topology.
- Improved the process blacklisting to ensure that only processes that are of interest to the user is reported to StackState.

## 2.0.8 (2019-12-20)

**Features**

- Cloudera Manager integration _[(STAC-6702)](https://stackstate.atlassian.net/browse/STAC-6702)_

## 2.0.7 (2019-12-17)

**Improvements**

- Enrich kubernetes topology information with the namespace as a label on all StackState components _[(STAC-7084)](https://stackstate.atlassian.net/browse/STAC-7084)_
- Cluster agent publishes phase information for Pods and adds another identifier to services that allows merging with trace services _[(STAC-6605)](https://stackstate.atlassian.net/browse/STAC-6605)_

**Bugs**

- Fix service identifiers that have no endpoint defined _[(STAC-7125)](https://stackstate.atlassian.net/browse/STAC-7125)_
- Do not include pod endpoint as identifier for the services _[(STAC-7248)](https://stackstate.atlassian.net/browse/STAC-7248)_

## 2.0.6 (2019-11-28)

**Features**

- Allow linux and windows install scripts to work offline and install a local downloaded package of the StackState Agent _[(STAC-5977)](https://stackstate.atlassian.net/browse/STAC-5977)_
- Support encryption for secrets in agent configurations using user-provided executable _[(STAC-6113)](https://stackstate.atlassian.net/browse/STAC-6113)_
- Extend cluster agent to gather high level components (controllers, jobs, volumes, ingresses) _[(STAC-5372)](https://stackstate.atlassian.net/browse/STAC-5372)_

**Improvements**

- Enrich kubernetes topology information to enable telemetry mapping _[(STAC-5373)](https://stackstate.atlassian.net/browse/STAC-5373)_

## 2.0.5 (2019-10-10)

**Features**

- Node agent reports cluster name in the connection namespace if present _[(STAC-5376)](https://stackstate.atlassian.net/browse/STAC-5376)_

  This feature allows the DNAT endpoint (which is observed looking at connections flowing through it) to be merged with the service gathered by the cluster agent.

- Make cluster agent gather OpenShift topology _[(STAC-5847)](https://stackstate.atlassian.net/browse/STAC-5847)_
- Enable new cluster agent to gather Kubernetes topology _[(STAC-5008)](https://stackstate.atlassian.net/browse/STAC-5008)_

**Improvements**

- Performance improvements for the stackstate agent _[(STAC-5968)](https://stackstate.atlassian.net/browse/STAC-5968)_
- Fixed agent and trace agent branding _[(STAC-5945)](https://stackstate.atlassian.net/browse/STAC-5945)_

## 2.0.4 (2019-08-26)

**Features**

- Add topology to python base check _[(STAC-4964)](https://stackstate.atlassian.net/browse/STAC-4964)_
- Add new stackstate-agent-integrations _[(STAC-4964)](https://stackstate.atlassian.net/browse/STAC-4964)_
- Add python bindings and handling of topology _[(STAC-4869)](https://stackstate.atlassian.net/browse/STAC-4869)_
- Enable new trace agent and propagate starttime, pid and hostname tags _[(STAC-4878)](https://stackstate.atlassian.net/browse/STAC-4878)_

**Bugs**

- Fix windows agent branding _[(STAC-3988)](https://stackstate.atlassian.net/browse/STAC-3988)_

## 2.0.3 (2019-05-28)

**Features**

- Filter reported processes _[(STAC-3401)](https://stackstate.atlassian.net/browse/STAC-3401)_

  This feature changed and extended the agent configuration.

  Under the `process_config` section we removed `blacklist_patterns` and introduced the following:

  ```
  process_blacklist:
    # A list of regex patterns that will exclude a process arguments if matched.
    patterns:
      - ...
    # Inclusions rules for blacklisted processes which reports high usage.
    inclusions:
      amount_top_cpu_pct_usage: 3
      cpu_pct_usage_threshold: 20
      amount_top_io_read_usage: 3
      amount_top_io_write_usage: 3
      amount_top_mem_usage: 3
      mem_usage_threshold: 35
  ```

  Those configurations can be provided through environment variables as well:

  | Parameter | Default | Description |
  |-----------|---------|-------------|
  | `STS_PROCESS_BLACKLIST_PATTERNS` | [see github](https://github.com/StackVista/stackstate-process-agent/blob/master/config/config_nix.go) | A list of regex patterns that will exclude a process if matched |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_TOP_CPU` | 0 | Number of processes to report that have a high CPU usage |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_TOP_IO_READ` | 0 | Number of processes to report that have a high IO read usage |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_TOP_IO_WRITE` | 0 | Number of processes to report that have a high IO write usage |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_TOP_MEM` | 0 | Number of processes to report that have a high Memory usage |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_CPU_THRESHOLD` |  | Threshold that enables the reporting of high CPU usage processes |
  | `STS_PROCESS_BLACKLIST_INCLUSIONS_MEM_THRESHOLD` |  | Threshold that enables the reporting of high Memory usage processes |

- Report localhost connections within the same network namespace _[(STAC-2891)](https://stackstate.atlassian.net/browse/STAC-2891)_

  This feature adds support to identify localhost connections within docker containers within the same network namespace.

  The network namespace of the reported connection can be observed in StackState on the connection between the components.

- Upstream upgrade to 6.10.2 _[(STAC-3220)](https://stackstate.atlassian.net/browse/STAC-3220)_

## 2.0.2 (2019-03-28)

**Improvements**

- Disable resource snaps collection _[(STAC-2915)](https://stackstate.atlassian.net/browse/STAC-2915)_
- Support CentOS 6 _[(STAC-4139)](https://stackstate.atlassian.net/browse/STAC-4139)_
