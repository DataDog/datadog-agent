name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

# Linux-specific dependencies
if linux_target?
  dependency 'procps-ng'
  dependency 'curl'
end
if !fips_mode?
  dependency 'secret-generic-connector' unless heroku_target?
end

dependency 'datadog-agent-data-plane' if linux_target? && !heroku_target?

if (linux_target? && !heroku_target?) || windows_target?
  build do
    command_on_repo_root "bazelisk run -- //deps/compile_policy:install --destdir=#{install_dir}"
  end
end

# Bundled cacerts file (is this a good idea?)
dependency 'cacerts'

# External agents
dependency 'jmxfetch'

# Used for memory profiling with the `status py` agent subcommand
dependency 'pympler'

dependency "systemd" if linux_target?

if linux_target? and !heroku_target? # system-probe dependency
  build do
    command_on_repo_root "bazelisk run -- @libpcap//:install --destdir=#{install_dir}/embedded"
  end
end

# Include traps db file in snmp.d/traps_db/
build do
    command_on_repo_root "bazelisk run -- //deps/snmp_traps:install --destdir=#{install_dir}"
end

dependency 'datadog-agent-integrations-py3'


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

build do
    # Delete empty folders that can still be present when building
    # without the omnibus cache.
    # When the cache gets used, git will transparently remove empty dirs for us
    # We do this here since we are done building our dependencies, but haven't
    # started creating the agent directories, which might be empty but that we
    # still want to keep
    command "find #{install_dir} -type d -empty -delete"
end
