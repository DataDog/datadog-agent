=============
Release Notes
=============

.. _Release Notes_dca-1.9.1_dca-1.9.x:

1.9.1
=====

.. _Release Notes_dca-1.9.1_dca-1.9.x_Prelude:

Prelude
-------

Released on: 2020-10-21
Pinned to datadog-agent v7.23.1: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7231>`_..

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
Pinned to datadog-agent v7.23.0: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7230--6230>`_..

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
Please refer to the `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7210--6210>`_.

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
Please refer to the `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7200--6200>`_.

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
Please refer to the `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#7170>`_.

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
Please refer to the `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/master/CHANGELOG.rst#6150>`_.

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
