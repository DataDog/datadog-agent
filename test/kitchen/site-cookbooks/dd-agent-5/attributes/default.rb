default['dd-agent-5']['api_key'] = nil
default['dd-agent-5']['url'] = nil

if node['platform_family'] == 'windows'
  default['dd-agent-5']['agent_package_retries'] = nil
  default['dd-agent-5']['agent_package_retry_delay'] = nil
  default['dd-agent-5']['windows_agent_checksum'] = nil
  default['dd-agent-5']['windows_version'] = nil
  default['dd-agent-5']['windows_agent_filename'] = "ddagent-cli-latest"
  default['dd-agent-5']['windows_agent_url'] = 'https://ddagent-windows-stable.s3.amazonaws.com/'
  default['dd-agent-5']['agent_install_options'] = nil
  default['dd-agent-5']['config_dir'] = "#{ENV['ProgramData']}/Datadog"
  default['dd-agent-5']['agent_name'] = 'DatadogAgent'
  default['dd-agent-5']['agent6_config_dir'] = "#{ENV['ProgramData']}/Datadog"
else
  default['dd-agent-5']['config_dir'] = '/etc/dd-agent'
  default['dd-agent-5']['working_dir'] = '/tmp/install-script/'
  default['dd-agent-5']['install_script_url'] = 'https://raw.githubusercontent.com/DataDog/dd-agent/master/packaging/datadog-agent/source/install_agent.sh'
end

# Enable the agent to start at boot
default['dd-agent-5']['agent_enable'] = true
default['dd-agent-5']['agent_start'] = true
default['dd-agent-5']['enable_trace_agent'] = true
default['dd-agent-5']['enable_process_agent'] = true

