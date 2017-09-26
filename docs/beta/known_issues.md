# Known Issues

There are some issues with the Beta Agent. We apologize for this, but it is a beta.
This document will be updated as these issues are resolved.

## Checks

Even if the new Agent fully supports Python checks, a number of those provided
by [integrations-core](https://github.com/DataDog/integrations-core) are not quite
ready yet. This is the list of checks that are expected to fail if run within the
beta Agent:

* kubernetes
* kubernetes_state
* docker_daemon
* vsphere

The Docker and Kubernetes checks in particular are being rewritten in Go to take
advantage to the new internal architecture of the Agent. Therefore the Python
versions will never work within Agent 6. The rewrite is not yet finished, but the
new `docker` check has basic functionalities, specific docs will be published soon.

Some methods in the `AgentCheck` class are not yet implemented. These include:

* `service_metadata`
* `get_service_metadata`
* `get_instance_proxy`

These methods in `AgentCheck` have not yet been implemented, but we have not yet
decided if we are going to implement them:

* `generate_historate_func`
* `generate_histogram_func`
* `stop`

## Systems

We do not yet build packages for the full gamut of systems that Agent 5 targets.
While some will be dropped as unsupported, others are simply not yet supported.
Beta is currently available on these platforms:

* Debian x86_64 version 6 and above
* RedHat/CentOS x86_64 version 6 and above
* Windows 64-bit
