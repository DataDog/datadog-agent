# StackState Agent v2 releases

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
