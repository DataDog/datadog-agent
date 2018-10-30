=============
Release Notes
=============

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
------------------
- Expose telemetry metrics with the Open Metrics format instead of expvar

Bug Fixes
---------
- add mutex logic and safe guards to avoid race condition in the Autoscalers Controller.

1.0.0-rc.3
==========
2018-10-15

Enhancement Notes
------------------
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
------------------

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
