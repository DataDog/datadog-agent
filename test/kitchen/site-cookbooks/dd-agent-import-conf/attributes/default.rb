default['dd-agent-import-conf']['api_key'] = nil

if node['platform_family'] == 'windows'
  default['dd-agent-import-conf']['config_dir'] = "#{ENV['ProgramData']}/Datadog"
  default['dd-agent-import-conf']['agent_name'] = 'DatadogAgent'
  default['dd-agent-import-conf']['agent6_config_dir'] = "#{ENV['ProgramData']}/Datadog"
else
  default['dd-agent-import-conf']['agent6_config_dir'] = '/etc/datadog-agent'
  default['dd-agent-import-conf']['config_dir'] = '/etc/dd-agent'
  default['dd-agent-import-conf']['agent_name'] = 'datadog-agent'
end

default['dd-agent-import-conf']['agent_start'] = true
