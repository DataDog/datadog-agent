name 'agent-dependencies'

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


