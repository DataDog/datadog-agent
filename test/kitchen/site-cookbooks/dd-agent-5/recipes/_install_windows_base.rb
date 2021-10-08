#
# Cookbook Name:: dd-agent-5
# Recipe:: _install_windows_base
#
# Copyright (C) 2019-present Datadog
#
# All rights reserved - Do Not Redistribute
#

package_retries = node['dd-agent-5']['agent_package_retries']
package_retry_delay = node['dd-agent-5']['agent_package_retry_delay']
dd_agent_version = node['dd-agent-5']['windows_version']
dd_agent_filename = node['dd-agent-5']['windows_agent_filename']

dd_agent_installer_basename = dd_agent_filename

temp_file_basename = ::File.join(Chef::Config[:file_cache_path], 'ddagent-cli').gsub(File::SEPARATOR, File::ALT_SEPARATOR || File::SEPARATOR)

dd_agent_installer = "#{dd_agent_installer_basename}.msi"
temp_file = "#{temp_file_basename}.msi"
installer_type = :msi
# Agent >= 5.12.0 installs per-machine by default, but specifying ALLUSERS=1 shouldn't affect the install
agent_install_options = node['dd-agent-5']['agent_install_options']
install_options = "/norestart ALLUSERS=1  #{agent_install_options}"
use_windows_package_resource = true

agent_config_file = ::File.join(node['dd-agent-5']['config_dir'], 'datadog.conf')

# Set the Agent service enable or disable
agent_enable = node['dd-agent-5']['agent_enable'] ? :enable : :disable
# Set the correct Agent startup action
agent_start = node['dd-agent-5']['agent_start'] ? :start : :stop

package 'Datadog Agent removal' do
  package_name 'Datadog Agent'
  action :nothing
end

source_url = node['dd-agent-5']['windows_agent_url']
if !source_url.end_with? '/'
  source_url += '/'
end
source_url += dd_agent_installer

  # Download the installer to a temp location
remote_file temp_file do
  source source_url
  checksum node['dd-agent-5']['windows_agent_checksum'] if node['dd-agent-5']['windows_agent_checksum']
  retries package_retries unless package_retries.nil?
  retry_delay package_retry_delay unless package_retry_delay.nil?
  # As of v1.37, the windows cookbook doesn't upgrade the package if a newer version is downloaded
  # As a workaround uninstall the package first if a new MSI is downloaded
  notifies :remove, 'package[Datadog Agent removal]', :immediately
end

execute "install-agent" do
  command "start /wait msiexec /log install.log /q /i #{temp_file} #{install_options}"
  status_out = `sc interrogate datadogagent 2>&1`
  puts status_out
  action :run
end

service 'datadog-agent' do
    service_name node['dd-agent-5']['agent_name']
    action [agent_enable, agent_start]
    supports :restart => true, :start => true, :stop => true
    subscribes :restart, "template[#{agent_config_file}]", :delayed unless node['dd-agent-5']['agent_start'] == false
    restart_command "powershell -Command \"restart-service -Force -Name datadogagent\""
    # HACK: the restart can fail when we hit systemd's restart limits (by default, 5 starts every 10 seconds)
    # To workaround this, retry once after 5 seconds, and a second time after 10 seconds
    retries 2
    retry_delay 5
  end
