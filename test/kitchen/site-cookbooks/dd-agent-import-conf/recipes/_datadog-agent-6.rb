#
# Cookbook Name:: dd-agent-import-conf
# Recipe:: datadog-agent-6
#
# Copyright 2011-present, Datadog
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# Fail here at converge time if no api_key is set
ruby_block 'datadog-api-key-unset' do
  block do
    raise "Set ['datadog']['api_key'] as an attribute or on the node's run_state to configure this node's Datadog Agent."
  end
  only_if { node['dd-agent-import-conf']['api_key'].nil? }
end
  
is_windows = node['platform_family'] == 'windows'

#
# Configures a basic agent
# To add integration-specific configurations, add 'datadog::config_name' to
# the node's run_list and set the relevant attributes
#
agent6_config_file = ::File.join(node['dd-agent-import-conf']['agent6_config_dir'], 'datadog.yaml')

# Common configuration
service_provider = nil
if (((node['platform'] == 'amazon' || node['platform_family'] == 'amazon') && node['platform_version'].to_i != 2) ||
    (node['platform'] == 'ubuntu' && node['platform_version'].to_f < 15.04) || # chef <11.14 doesn't use the correct service provider
    (node['platform'] != 'amazon' && node['platform_family'] == 'rhel' && node['platform_version'].to_i < 7))
  # use Upstart provider explicitly for Agent 6 on Amazon Linux < 2.0 and RHEL < 7
  service_provider = Chef::Provider::Service::Upstart
end

service 'datadog-agent-6' do
  service_name node['dd-agent-import-conf']['agent_name']
  action :nothing
  provider service_provider unless service_provider.nil?
  if is_windows
    supports :restart => true, :start => true, :stop => true
    restart_command "powershell restart-service #{node['dd-agent-import-conf']['agent_name']} -Force"
    stop_command "powershell stop-service #{node['dd-agent-import-conf']['agent_name']} -Force"
  else
    supports :restart => true, :status => true, :start => true, :stop => true
  end
  subscribes :restart, "template[#{agent6_config_file}]", :delayed unless node['dd-agent-import-conf']['agent_start'] == false
  # HACK: the restart can fail when we hit systemd's restart limits (by default, 5 starts every 10 seconds)
  # To workaround this, retry once after 5 seconds, and a second time after 10 seconds
  retries 2
  retry_delay 5
end

# TODO: Add this when we update our datadog Berksfile dependency to 2.20 or 3.0
# only load system-probe recipe if an agent6 installation comes with it
# ruby_block 'include system-probe' do
#   block do
#     if ::File.exist?('/opt/datadog-agent/embedded/bin/system-probe') && !is_windows
#       run_context.include_recipe 'datadog::system-probe'
#     end
#   end
# end

# Install integration packages
#include_recipe 'datadog::integrations' unless is_windows
