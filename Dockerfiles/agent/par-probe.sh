#!/bin/sh
set -e

cli=/opt/datadog-agent/embedded/bin/dd-procmgr

"${cli}" status --json | grep -Eq '"ready":[[:space:]]*true'
"${cli}" describe --json datadog-agent-par-control | grep -Eq '"state":[[:space:]]*"Running"'
