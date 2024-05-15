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

# version manifest file
dependency 'version-manifest'


