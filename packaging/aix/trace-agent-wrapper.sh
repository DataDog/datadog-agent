#!/bin/sh
export LIBPATH=/opt/datadog-agent/rtloader:/opt/datadog-agent/embedded/lib:/opt/mqm/lib64:/opt/mqm/lib:/usr/mqm/lib64:/usr/mqm/lib:/opt/ibm/db2/clidriver/lib:/opt/datadog-agent/embedded/lib/python3.13/site-packages/clidriver/lib:/opt/freeware/lib64:/opt/freeware/lib${LIBPATH:+:$LIBPATH}
export PATH=/opt/datadog-agent/embedded/bin:/opt/freeware/bin:/usr/sbin:/usr/bin:/bin:"${PATH}"
exec /opt/datadog-agent/embedded/bin/trace-agent-bin "$@"
