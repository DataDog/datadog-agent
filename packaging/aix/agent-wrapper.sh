#!/bin/sh
# Agent entry point wrapper.
#
# AIX's dynamic loader uses LIBPATH (not RPATH) for library resolution.
# The agent binary's loader section has the build-host staging path baked in
# (a Go/ld limitation on AIX); this wrapper sets the correct installed paths
# so the binary works when invoked directly, not just via SRC.
export LIBPATH=/opt/datadog-agent/rtloader:/opt/datadog-agent/embedded/lib:/opt/mqm/lib64:/opt/mqm/lib:/usr/mqm/lib64:/usr/mqm/lib:/opt/ibm/db2/clidriver/lib:/opt/datadog-agent/embedded/lib/python3.13/site-packages/clidriver/lib:/opt/freeware/lib64:/opt/freeware/lib${LIBPATH:+:$LIBPATH}
export PATH=/opt/datadog-agent/embedded/bin:/opt/freeware/bin:/usr/sbin:/usr/bin:/bin:"${PATH}"
exec /opt/datadog-agent/bin/agent/agent-bin "$@"
