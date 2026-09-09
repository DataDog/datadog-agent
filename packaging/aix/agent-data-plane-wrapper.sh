#!/bin/sh
# Agent Data Plane (ADP) entry point wrapper.
#
# AIX's dynamic loader uses LIBPATH (not RPATH) for library resolution.
# The SRC subsystem invokes this binary directly (no shell), so without this
# wrapper LIBPATH is never set and the loader cannot find bundled libraries
# such as libunwind, causing an immediate "Cannot load program" failure.
export LIBPATH=/opt/datadog-agent/rtloader:/opt/datadog-agent/embedded/lib:/opt/freeware/lib64:/opt/freeware/lib${LIBPATH:+:$LIBPATH}
export PATH=/opt/datadog-agent/embedded/bin:/opt/freeware/bin:/usr/sbin:/usr/bin:/bin:"${PATH}"
exec /opt/datadog-agent/embedded/bin/agent-data-plane-bin "$@"
