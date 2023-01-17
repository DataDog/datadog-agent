#
# Linux options (Linux uses the official datadog cookbook)
#

default['datadog']['agent_start'] = true
default['datadog']['agent_enable'] = true
default['datadog']['agent_version'] = nil

# All other options use the defaults set in the official cookbook,
# or the options set in the kitchen job (eg. aptrepo, yumrepo, etc.)

#
# Windows options (Windows uses the custom dd-agent-install cookbook)
#

# The dd-agent-install recipe is a copy of the official install recipe,
# the only difference being the command used to install the Agent.
# Here, we add a start /wait to the command, otherwise chef doesn't wait 
# for the Agent to be installed before ending its run.
# This behavior could make kitchen tests fail, because we would start testing
# the Agent before it is ready.

default['dd-agent-install']['api_key'] = nil
default['dd-agent-install']['agent_major_version'] = nil
default['dd-agent-install']['windows_version'] = nil # => install the latest available version
default['dd-agent-install']['windows_agent_checksum'] = nil
default['dd-agent-install']['windows_agent_url'] = 'https://ddagent-windows-stable.s3.amazonaws.com/'

default['dd-agent-install']['agent_package_retries'] = nil
default['dd-agent-install']['agent_package_retry_delay'] = nil
default['dd-agent-install']['config_dir'] = "#{ENV['ProgramData']}/Datadog"
default['dd-agent-install']['agent_name'] = 'DatadogAgent'
default['dd-agent-install']['agent6_config_dir'] = "#{ENV['ProgramData']}/Datadog"

# Enable the agent to start at boot
default['dd-agent-install']['agent_enable'] = true

# Start agent or not
default['dd-agent-install']['agent_start'] = true
default['dd-agent-install']['enable_trace_agent'] = true
default['dd-agent-install']['enable_process_agent'] = true

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
default['dd-agent-install']['log_file_directory'] = nil
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
