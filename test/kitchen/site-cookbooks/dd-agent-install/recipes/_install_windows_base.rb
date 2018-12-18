#
# Cookbook Name:: dd-agent-install
# Recipe:: _install_windows_base
#
# Copyright (C) 2018 Datadog
#
# All rights reserved - Do Not Redistribute
#

package_retries = node['dd-agent-install']['agent_package_retries']
package_retry_delay = node['dd-agent-install']['agent_package_retry_delay']
dd_agent_version = node['dd-agent-install']['windows_version']

if dd_agent_version
  dd_agent_installer_basename = "datadog-agent-#{dd_agent_version}-1-x86_64"
else
  dd_agent_installer_basename = "datadog-agent-6.0.0-beta.latest.amd64"
end

temp_file_basename = ::File.join(Chef::Config[:file_cache_path], 'ddagent-cli')

dd_agent_installer = "#{dd_agent_installer_basename}.msi"
temp_file = "#{temp_file_basename}.msi"
installer_type = :msi
# Agent >= 5.12.0 installs per-machine by default, but specifying ALLUSERS=1 shouldn't affect the install
agent_install_options = node['dd-agent-install']['agent_install_options']
install_options = "/norestart ALLUSERS=1  #{agent_install_options}"
use_windows_package_resource = true

package 'Datadog Agent removal' do
  package_name 'Datadog Agent'
  action :nothing
end

source_url = node['dd-agent-install']['windows_agent_url']
if !source_url.end_with? '/'
  source_url += '/'
end
source_url += dd_agent_installer

  # Download the installer to a temp location
remote_file temp_file do
  source source_url
  checksum node['dd-agent-install']['windows_agent_checksum'] if node['dd-agent-install']['windows_agent_checksum']
  retries package_retries unless package_retries.nil?
  retry_delay package_retry_delay unless package_retry_delay.nil?
  # As of v1.37, the windows cookbook doesn't upgrade the package if a newer version is downloaded
  # As a workaround uninstall the package first if a new MSI is downloaded
  notifies :remove, 'package[Datadog Agent removal]', :immediately
end

execute "install-agent" do
  command "start /wait #{temp_file} #{install_options}"
  action :run
end