name 'agent-dependencies'

# Linux-specific dependencies
if linux_target?
  dependency 'procps-ng'
  dependency 'curl'
end

# Bundled cacerts file (is this a good idea?)
dependency 'cacerts'

# External agents
dependency 'jmxfetch'

if linux_target? || osx_target?
  dependency 'sds'
end

# version manifest file
dependency 'version-manifest'


