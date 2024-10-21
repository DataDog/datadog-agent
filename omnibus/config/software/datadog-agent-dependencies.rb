name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

# Used for memory profiling with the `status py` agent subcommand
dependency 'pympler'

dependency 'datadog-agent-integrations-py3-dependencies'

dependency "systemd" if linux_target?

dependency 'libpcap' if linux_target? and !heroku_target? # system-probe dependency