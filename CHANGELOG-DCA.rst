=============
Release Notes
=============

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
- Move some logs info to debug, in order to not generates useless logs in normal case.

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
