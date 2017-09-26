# Known Issues

There are some issues with the Beta Agent. We apologize for this, but it is a beta. This document will be updated as these issues are resolved.

## Checks

The Docker and Kubernetes are being rewritten in go to take advantage to the new internal architecture of the agent. Therefore the python version won't work within Agent 6. The rewrite is not yet finished, however.

Some methods in the `AgentCheck` class are not yet implemented. These include:

* `service_metadata`
* `get_service_metadata`
* `get_instance_proxy`

These methods in `AgentCheck` have not yet been implemented, but we have not yet decided if we are going to implement them:

* `generate_historate_func`
* `generate_histogram_func`
* `stop`

## Systems

We do not yet build packages for the full gamut of systems that Agent 5 targets. While some are being dropped as unsupported, others are simply not yet supported.

CentOS 5 is not supported.
We are not yet building a package to target SUSE Enterprise Linux.
