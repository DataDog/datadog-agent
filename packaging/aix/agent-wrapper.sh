#!/bin/sh
# Agent entry point wrapper.
#
# AIX's dynamic loader uses LIBPATH (not RPATH) for library resolution.
# The agent binary's loader section has the build-host staging path baked in
# (a Go/ld limitation on AIX); this wrapper sets the correct installed paths
# so the binary works when invoked directly, not just via SRC.
export LIBPATH=/opt/datadog-agent/rtloader:/opt/datadog-agent/embedded/lib:/opt/mqm/lib64:/opt/mqm/lib:/usr/mqm/lib64:/usr/mqm/lib:/opt/ibm/db2/clidriver/lib:/opt/datadog-agent/embedded/lib/python3.13/site-packages/clidriver/lib:/opt/freeware/lib64:/opt/freeware/lib${LIBPATH:+:$LIBPATH}
# The IBM MQ client library looks up its own message catalogs via NLSPATH;
# without this, MQ errors render as unreadable generic text (e.g. AMQ9211E
# "Failed to find error message id") instead of the real message.
export NLSPATH=/opt/mqm/msg/%L/%N:/opt/mqm/msg/%L/%N.cat:/usr/mqm/msg/%L/%N:/usr/mqm/msg/%L/%N.cat${NLSPATH:+:$NLSPATH}
export PATH=/opt/datadog-agent/embedded/bin:/opt/freeware/bin:/usr/sbin:/usr/bin:/bin:"${PATH}"
exec /opt/datadog-agent/bin/agent/agent-bin "$@"
