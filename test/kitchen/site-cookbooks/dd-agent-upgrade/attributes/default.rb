default['dd-agent-upgrade']['api_key'] = nil
default['dd-agent-upgrade']['agent_major_version'] = nil

default['dd-agent-upgrade']['version'] = nil # => install the latest available version
default['dd-agent-upgrade']['add_new_repo'] = false # If set to true, be sure to set aptrepo and yumrepo
default['dd-agent-upgrade']['aptrepo'] =  nil
default['dd-agent-upgrade']['aptrepo_dist'] =  nil
default['dd-agent-upgrade']['yumrepo'] = nil
default['dd-agent-upgrade']['yumrepo_suse'] = nil
default['dd-agent-upgrade']['package_name'] = 'datadog-agent'

default['dd-agent-upgrade']['windows_version'] = nil # => install the latest available version
default['dd-agent-upgrade']['windows_agent_checksum'] = nil
default['dd-agent-upgrade']['windows_agent_url'] = 'https://ddagent-windows-stable.s3.amazonaws.com/'

default['dd-agent-upgrade']['agent_package_retries'] = nil
default['dd-agent-upgrade']['agent_package_retry_delay'] = nil

# Enable the agent to start at boot
default['dd-agent-upgrade']['agent_enable'] = true

# Start agent or not
default['dd-agent-upgrade']['agent_start'] = true
default['dd-agent-upgrade']['enable_trace_agent'] = true
default['dd-agent-upgrade']['enable_process_agent'] = true

# Set the defaults from the chef recipe
default['dd-agent-upgrade']['extra_endpoints']['prod']['enabled'] = nil
default['dd-agent-upgrade']['extra_endpoints']['prod']['api_key'] = nil
default['dd-agent-upgrade']['extra_endpoints']['prod']['application_key'] = nil
default['dd-agent-upgrade']['extra_endpoints']['prod']['url'] = nil # op
default['dd-agent-upgrade']['extra_config']['forwarder_timeout'] = nil
default['dd-agent-upgrade']['web_proxy']['host'] = nil
default['dd-agent-upgrade']['web_proxy']['port'] = nil
default['dd-agent-upgrade']['web_proxy']['user'] = nil
default['dd-agent-upgrade']['web_proxy']['password'] = nil
default['dd-agent-upgrade']['web_proxy']['skip_ssl_validation'] = nil # accepted values 'yes' or 'no'
default['dd-agent-upgrade']['dogstreams'] = []
default['dd-agent-upgrade']['custom_emitters'] = []
default['dd-agent-upgrade']['syslog']['active'] = false
default['dd-agent-upgrade']['syslog']['udp'] = false
default['dd-agent-upgrade']['syslog']['host'] = nil
default['dd-agent-upgrade']['syslog']['port'] = nil
default['dd-agent-upgrade']['log_file_directory'] =
  if node['platform_family'] == 'windows'
    nil # let the agent use a default log file dir
  else
    '/var/log/datadog'
  end
default['dd-agent-upgrade']['process_agent']['blacklist'] = nil
default['dd-agent-upgrade']['process_agent']['container_blacklist'] = nil
default['dd-agent-upgrade']['process_agent']['container_whitelist'] = nil
default['dd-agent-upgrade']['process_agent']['log_file'] = nil
default['dd-agent-upgrade']['process_agent']['process_interval'] = nil
default['dd-agent-upgrade']['process_agent']['rtprocess_interval'] = nil
default['dd-agent-upgrade']['process_agent']['container_interval'] = nil
default['dd-agent-upgrade']['process_agent']['rtcontainer_interval'] = nil
default['dd-agent-upgrade']['tags'] = ''
default['dd-agent-upgrade']['histogram_aggregates'] = 'max, median, avg, count'
default['dd-agent-upgrade']['histogram_percentiles'] = '0.95'
default['dd-agent-upgrade']['dogstatsd'] = true
default['dd-agent-upgrade']['dogstatsd_port'] = 8125
default['dd-agent-upgrade']['dogstatsd_interval'] = 10
default['dd-agent-upgrade']['dogstatsd_normalize'] = 'yes'
default['dd-agent-upgrade']['dogstatsd_target'] = 'http://localhost:17123'
default['dd-agent-upgrade']['statsd_forward_host'] = nil
default['dd-agent-upgrade']['statsd_forward_port'] = 8125
default['dd-agent-upgrade']['statsd_metric_namespace'] = nil
default['dd-agent-upgrade']['log_level'] = 'INFO'
default['dd-agent-upgrade']['enable_logs_agent'] = false
