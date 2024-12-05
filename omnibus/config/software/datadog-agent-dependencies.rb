name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

# Linux-specific dependencies
if linux_target?
  dependency 'procps-ng'
  dependency 'curl'
  if fips_mode?
    dependency 'openssl-fips-provider'
  end
end

# Bundled cacerts file (is this a good idea?)
dependency 'cacerts'

# External agents
dependency 'jmxfetch'

if linux_target?
  dependency 'sds'
end

# version manifest file
dependency 'version-manifest'

# Used for memory profiling with the `status py` agent subcommand
dependency 'pympler'

dependency 'datadog-agent-integrations-py3-dependencies'

dependency "systemd" if linux_target?

dependency 'libpcap' if linux_target? and !heroku_target? # system-probe dependency
