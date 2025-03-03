name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

# Linux-specific dependencies
if linux_target?
  dependency 'procps-ng'
  dependency 'curl'
end
if fips_mode?
  dependency 'openssl-fips-provider'
end

# Bundled cacerts file (is this a good idea?)
dependency 'cacerts'

# External agents
dependency 'jmxfetch'

if linux_target?
  dependency 'sds'
end

# Used for memory profiling with the `status py` agent subcommand
dependency 'pympler'

dependency 'datadog-agent-integrations-py3-dependencies'

dependency "systemd" if linux_target?

dependency 'libpcap' if linux_target? and !heroku_target? # system-probe dependency

# Include traps db file in snmp.d/traps_db/
dependency 'snmp-traps'

# Additional software
if windows_target?
  if ENV['WINDOWS_DDNPM_DRIVER'] and not ENV['WINDOWS_DDNPM_DRIVER'].empty?
    dependency 'datadog-windows-filter-driver'
  end
  if ENV['WINDOWS_APMINJECT_MODULE'] and not ENV['WINDOWS_APMINJECT_MODULE'].empty?
    dependency 'datadog-windows-apminject'
  end
  if ENV['WINDOWS_DDPROCMON_DRIVER'] and not ENV['WINDOWS_DDPROCMON_DRIVER'].empty?
    dependency 'datadog-windows-procmon-driver'
  end
end

