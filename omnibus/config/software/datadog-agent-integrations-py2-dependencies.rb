name 'datadog-agent-integrations-py2-dependencies'

dependency 'pip2'

if linux_target?
  # add nfsiostat script
  dependency 'nfsiostat'
  # needed for glusterfs
  dependency 'gstatus'
end
