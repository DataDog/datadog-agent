default['dd-agent-reinstall']['api_key'] = nil
default['dd-agent-reinstall']['agent_major_version'] = nil

default['dd-agent-reinstall']['version'] = nil # => install the latest available version
default['dd-agent-reinstall']['add_new_repo'] = false # If set to true, be sure to set aptrepo and yumrepo
default['dd-agent-reinstall']['aptrepo'] =  nil
default['dd-agent-reinstall']['aptrepo_dist'] =  nil
default['dd-agent-reinstall']['yumrepo'] = nil
default['dd-agent-reinstall']['yumrepo_suse'] = nil
default['dd-agent-reinstall']['package_name'] = 'datadog-agent'

default['dd-agent-reinstall']['windows_version'] = nil # => install the latest available version
default['dd-agent-reinstall']['windows_agent_checksum'] = nil
default['dd-agent-reinstall']['windows_agent_url'] = 'https://ddagent-windows-stable.s3.amazonaws.com/'

default['dd-agent-reinstall']['agent_package_retries'] = nil
default['dd-agent-reinstall']['agent_package_retry_delay'] = nil

# Enable the agent to start at boot
default['dd-agent-reinstall']['agent_enable'] = true

# Start agent or not
default['dd-agent-reinstall']['agent_start'] = true
default['dd-agent-reinstall']['enable_trace_agent'] = true
default['dd-agent-reinstall']['enable_process_agent'] = true

# Set the defaults from the chef recipe
default['dd-agent-reinstall']['extra_endpoints']['prod']['enabled'] = nil
default['dd-agent-reinstall']['extra_endpoints']['prod']['api_key'] = nil
default['dd-agent-reinstall']['extra_endpoints']['prod']['application_key'] = nil
default['dd-agent-reinstall']['extra_endpoints']['prod']['url'] = nil # op
default['dd-agent-reinstall']['extra_config']['forwarder_timeout'] = nil
default['dd-agent-reinstall']['web_proxy']['host'] = nil
default['dd-agent-reinstall']['web_proxy']['port'] = nil
default['dd-agent-reinstall']['web_proxy']['user'] = nil
default['dd-agent-reinstall']['web_proxy']['password'] = nil
default['dd-agent-reinstall']['web_proxy']['skip_ssl_validation'] = nil # accepted values 'yes' or 'no'
default['dd-agent-reinstall']['dogstreams'] = []
default['dd-agent-reinstall']['custom_emitters'] = []
default['dd-agent-reinstall']['syslog']['active'] = false
default['dd-agent-reinstall']['syslog']['udp'] = false
default['dd-agent-reinstall']['syslog']['host'] = nil
default['dd-agent-reinstall']['syslog']['port'] = nil
default['dd-agent-reinstall']['log_file_directory'] =
  if node['platform_family'] == 'windows'
    nil # let the agent use a default log file dir
  else
    '/var/log/datadog'
  end
default['dd-agent-reinstall']['process_agent']['blacklist'] = nil
default['dd-agent-reinstall']['process_agent']['container_blacklist'] = nil
default['dd-agent-reinstall']['process_agent']['container_whitelist'] = nil
default['dd-agent-reinstall']['process_agent']['log_file'] = nil
default['dd-agent-reinstall']['process_agent']['process_interval'] = nil
default['dd-agent-reinstall']['process_agent']['rtprocess_interval'] = nil
default['dd-agent-reinstall']['process_agent']['container_interval'] = nil
default['dd-agent-reinstall']['process_agent']['rtcontainer_interval'] = nil
default['dd-agent-reinstall']['tags'] = ''
default['dd-agent-reinstall']['histogram_aggregates'] = 'max, median, avg, count'
default['dd-agent-reinstall']['histogram_percentiles'] = '0.95'
default['dd-agent-reinstall']['dogstatsd'] = true
default['dd-agent-reinstall']['dogstatsd_port'] = 8125
default['dd-agent-reinstall']['dogstatsd_interval'] = 10
default['dd-agent-reinstall']['dogstatsd_normalize'] = 'yes'
default['dd-agent-reinstall']['dogstatsd_target'] = 'http://localhost:17123'
default['dd-agent-reinstall']['statsd_forward_host'] = nil
default['dd-agent-reinstall']['statsd_forward_port'] = 8125
default['dd-agent-reinstall']['statsd_metric_namespace'] = nil
default['dd-agent-reinstall']['log_level'] = 'INFO'
default['dd-agent-reinstall']['enable_logs_agent'] = false
