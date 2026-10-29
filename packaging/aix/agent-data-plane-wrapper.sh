#!/bin/sh
# Agent Data Plane (ADP) entry point wrapper.
#
# AIX's dynamic loader uses LIBPATH (not RPATH) for library resolution.
# The SRC subsystem invokes this binary directly (no shell), so without this
# wrapper LIBPATH is never set and the loader cannot find bundled libraries
# such as libunwind, causing an immediate "Cannot load program" failure.
export LIBPATH=/opt/datadog-agent/rtloader:/opt/datadog-agent/embedded/lib:/opt/freeware/lib64:/opt/freeware/lib${LIBPATH:+:$LIBPATH}
export PATH=/opt/datadog-agent/embedded/bin:/opt/freeware/bin:/usr/sbin:/usr/bin:/bin:"${PATH}"

# Raise the data-segment soft limit to 512MiB (1048576 blocks of 512 bytes)
# when it is lower. agent-data-plane allocates its heap through libc malloc
# and is bound by RLIMIT_DATA: against the AIX 128MiB default, malloc fails
# and the process aborts. The postinst raises the dd-agent user's limit in
# /etc/security/limits, but whether SRC applies that stanza to subsystems is
# undocumented — raising here covers that, and a process may raise its own
# soft limit up to the hard limit without privileges. Never lower a higher
# or unlimited limit.
# shellcheck disable=SC3045  # ulimit -d is supported by AIX sh (not POSIX sh)
_dd_ulimit=$(ulimit -d 2>/dev/null)
if [ "$_dd_ulimit" != unlimited ] && [ "$_dd_ulimit" -lt 1048576 ] 2>/dev/null; then
    # shellcheck disable=SC3045
    ulimit -d 1048576 2>/dev/null \
        || echo "agent-data-plane: failed to raise data-segment limit to 512MiB (check data_hard in /etc/security/limits)" >&2
fi
unset _dd_ulimit

exec /opt/datadog-agent/embedded/bin/agent-data-plane-bin "$@"
