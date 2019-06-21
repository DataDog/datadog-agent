default['dd-agent-5']['api_key'] = nil
default['dd-agent-5']['version'] = nil # => install the latest available version
default['dd-agent-upgrade']['windows_version'] = nil # => install the latest available version
default['dd-agent-5']['add_new_repo'] = false # If set to true, be sure to set aptrepo and yumrepo
default['dd-agent-5']['aptrepo'] =  nil
default['dd-agent-5']['yumrepo'] = nil
default['dd-agent-5']['package_name'] = 'datadog-agent'
default['dd-agent-5']['version'] = nil
default['dd-agent-5']['agent6'] = nil
default['dd-agent-5']['windows_agent_checksum'] = nil
default['dd-agent-5']['windows_agent_filename'] = "ddagent-cli-latest"
default['dd-agent-5']['windows_agent_url'] = 'https://s3.amazonaws.com/ddagent-windows-stable/'

default['dd-agent-5']['agent_package_retries'] = nil
default['dd-agent-5']['agent_package_retry_delay'] = nil
if node['platform_family'] == 'windows'
  default['dd-agent-5']['config_dir'] = "#{ENV['ProgramData']}/Datadog"
  default['dd-agent-5']['agent_name'] = 'DatadogAgent'
  default['dd-agent-5']['agent6_config_dir'] = "#{ENV['ProgramData']}/Datadog"
  # Some settings from the chef recipe it needs
  default['datadog']['config_dir'] = "#{ENV['ProgramData']}/Datadog"
  default['datadog']['agent_name'] = 'DatadogAgent'
  default['datadog']['agent6_config_dir'] = "#{ENV['ProgramData']}/Datadog"
else
  default['dd-agent-5']['config_dir'] = '/etc/dd-agent'
  default['dd-agent-5']['agent_name'] = 'datadog-agent'
end
# Enable the agent to start at boot
default['dd-agent-5']['agent_enable'] = true

# Start agent or not
default['dd-agent-5']['agent_start'] = true
default['dd-agent-5']['enable_trace_agent'] = true
default['dd-agent-5']['enable_process_agent'] = true

default['dd-agent-5']['working_dir'] = '/tmp/install-script/'
default['dd-agent-5']['install_script_url'] = 'https://raw.githubusercontent.com/DataDog/dd-agent/master/packaging/datadog-agent/source/install_agent.sh'

default['datadog']['agent_start'] = true
default['datadog']['agent_enable'] = true

# Set the defaults from the chef recipe
default['dd-agent-5']['extra_endpoints']['prod']['enabled'] = nil
default['dd-agent-5']['extra_endpoints']['prod']['api_key'] = nil
default['dd-agent-5']['extra_endpoints']['prod']['application_key'] = nil
default['dd-agent-5']['extra_endpoints']['prod']['url'] = nil # op
default['dd-agent-5']['extra_config']['forwarder_timeout'] = nil
default['dd-agent-5']['web_proxy']['host'] = nil
default['dd-agent-5']['web_proxy']['port'] = nil
default['dd-agent-5']['web_proxy']['user'] = nil
default['dd-agent-5']['web_proxy']['password'] = nil
default['dd-agent-5']['web_proxy']['skip_ssl_validation'] = nil # accepted values 'yes' or 'no'
default['dd-agent-5']['dogstreams'] = []
default['dd-agent-5']['custom_emitters'] = []
default['dd-agent-5']['syslog']['active'] = false
default['dd-agent-5']['syslog']['udp'] = false
default['dd-agent-5']['syslog']['host'] = nil
default['dd-agent-5']['syslog']['port'] = nil
default['dd-agent-5']['log_file_directory'] =
  if node['platform_family'] == 'windows'
    nil # let the agent use a default log file dir
  else
    '/var/log/datadog'
  end
default['dd-agent-5']['process_agent']['blacklist'] = nil
default['dd-agent-5']['process_agent']['container_blacklist'] = nil
default['dd-agent-5']['process_agent']['container_whitelist'] = nil
default['dd-agent-5']['process_agent']['log_file'] = nil
default['dd-agent-5']['process_agent']['process_interval'] = nil
default['dd-agent-5']['process_agent']['rtprocess_interval'] = nil
default['dd-agent-5']['process_agent']['container_interval'] = nil
default['dd-agent-5']['process_agent']['rtcontainer_interval'] = nil
default['dd-agent-5']['tags'] = ''
default['dd-agent-5']['histogram_aggregates'] = 'max, median, avg, count'
default['dd-agent-5']['histogram_percentiles'] = '0.95'
default['dd-agent-5']['dogstatsd'] = true
default['dd-agent-5']['dogstatsd_port'] = 8125
default['dd-agent-5']['dogstatsd_interval'] = 10
default['dd-agent-5']['dogstatsd_normalize'] = 'yes'
default['dd-agent-5']['dogstatsd_target'] = 'http://localhost:17123'
default['dd-agent-5']['statsd_forward_host'] = nil
default['dd-agent-5']['statsd_forward_port'] = 8125
default['dd-agent-5']['statsd_metric_namespace'] = nil
default['dd-agent-5']['log_level'] = 'INFO'
default['dd-agent-5']['enable_logs_agent'] = false

default['dd-agent-5']['agent_install_options'] = ''
