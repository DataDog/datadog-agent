default['dd-agent-install']['api_key'] = nil
default['dd-agent-install']['version'] = nil # => install the latest available version
default['dd-agent-install']['windows_version'] = nil # => install the latest available version
default['dd-agent-install']['add_new_repo'] = false # If set to true, be sure to set aptrepo and yumrepo
default['dd-agent-install']['aptrepo'] =  nil
default['dd-agent-install']['yumrepo'] = nil
default['dd-agent-install']['package_name'] = 'datadog-agent'
default['dd-agent-install']['version'] = nil
default['dd-agent-install']['agent_major_version'] = nil
default['dd-agent-install']['windows_agent_checksum'] = nil
default['dd-agent-install']['windows_agent_url'] = 'https://s3.amazonaws.com/ddagent-windows-stable/'

default['dd-agent-install']['agent_package_retries'] = nil
default['dd-agent-install']['agent_package_retry_delay'] = nil
if node['platform_family'] == 'windows'
  default['dd-agent-install']['config_dir'] = "#{ENV['ProgramData']}/Datadog"
  default['dd-agent-install']['agent_name'] = 'DatadogAgent'
  default['dd-agent-install']['agent6_config_dir'] = "#{ENV['ProgramData']}/Datadog"
  # Some settings from the chef recipe it needs
  default['datadog']['config_dir'] = "#{ENV['ProgramData']}/Datadog"
  default['datadog']['agent_name'] = 'DatadogAgent'
  default['datadog']['agent6_config_dir'] = "#{ENV['ProgramData']}/Datadog"
else
  default['dd-agent-install']['config_dir'] = '/etc/dd-agent'
  default['dd-agent-install']['agent_name'] = 'datadog-agent'
end
# Enable the agent to start at boot
default['dd-agent-install']['agent_enable'] = true

# Start agent or not
default['dd-agent-install']['agent_start'] = true
default['dd-agent-install']['enable_trace_agent'] = true
default['dd-agent-install']['enable_process_agent'] = true

default['datadog']['agent_start'] = true
default['datadog']['agent_enable'] = true

# Set the defaults from the chef recipe
default['dd-agent-install']['extra_endpoints']['prod']['enabled'] = nil
default['dd-agent-install']['extra_endpoints']['prod']['api_key'] = nil
default['dd-agent-install']['extra_endpoints']['prod']['application_key'] = nil
default['dd-agent-install']['extra_endpoints']['prod']['url'] = nil # op
default['dd-agent-install']['extra_config']['forwarder_timeout'] = nil
default['dd-agent-install']['web_proxy']['host'] = nil
default['dd-agent-install']['web_proxy']['port'] = nil
default['dd-agent-install']['web_proxy']['user'] = nil
default['dd-agent-install']['web_proxy']['password'] = nil
default['dd-agent-install']['web_proxy']['skip_ssl_validation'] = nil # accepted values 'yes' or 'no'
default['dd-agent-install']['dogstreams'] = []
default['dd-agent-install']['custom_emitters'] = []
default['dd-agent-install']['syslog']['active'] = false
default['dd-agent-install']['syslog']['udp'] = false
default['dd-agent-install']['syslog']['host'] = nil
default['dd-agent-install']['syslog']['port'] = nil
default['dd-agent-install']['log_file_directory'] =
  if node['platform_family'] == 'windows'
    nil # let the agent use a default log file dir
  else
    '/var/log/datadog'
  end
default['dd-agent-install']['process_agent']['blacklist'] = nil
default['dd-agent-install']['process_agent']['container_blacklist'] = nil
default['dd-agent-install']['process_agent']['container_whitelist'] = nil
default['dd-agent-install']['process_agent']['log_file'] = nil
default['dd-agent-install']['process_agent']['process_interval'] = nil
default['dd-agent-install']['process_agent']['rtprocess_interval'] = nil
default['dd-agent-install']['process_agent']['container_interval'] = nil
default['dd-agent-install']['process_agent']['rtcontainer_interval'] = nil
default['dd-agent-install']['tags'] = ''
default['dd-agent-install']['histogram_aggregates'] = 'max, median, avg, count'
default['dd-agent-install']['histogram_percentiles'] = '0.95'
default['dd-agent-install']['dogstatsd'] = true
default['dd-agent-install']['dogstatsd_port'] = 8125
default['dd-agent-install']['dogstatsd_interval'] = 10
default['dd-agent-install']['dogstatsd_normalize'] = 'yes'
default['dd-agent-install']['dogstatsd_target'] = 'http://localhost:17123'
default['dd-agent-install']['statsd_forward_host'] = nil
default['dd-agent-install']['statsd_forward_port'] = 8125
default['dd-agent-install']['statsd_metric_namespace'] = nil
default['dd-agent-install']['log_level'] = 'INFO'
default['dd-agent-install']['enable_logs_agent'] = false

default['dd-agent-install']['agent_install_options'] = ''
