#
# Cookbook Name:: dd-agent-install
# Recipe:: _install_windows_base
#
# Copyright (C) 2019-present Datadog
#
# All rights reserved - Do Not Redistribute
#

package_retries = node['dd-agent-install']['agent_package_retries']
package_retry_delay = node['dd-agent-install']['agent_package_retry_delay']
dd_agent_version = node['datadog']['agent_version'] || node['dd-agent-install']['windows_version']
dd_agent_filename = node['dd-agent-install']['windows_agent_filename']

source_url = node['dd-agent-install']['windows_agent_url']
if !source_url.end_with? '/'
  source_url += '/'
end

if dd_agent_filename
  dd_agent_installer_basename = dd_agent_filename
else
  # HACK: the packages have different names in the stable repos and the testing repos
  # Check the source URL to know if we need to use the "stable" filename, or the "testing" filename
  if source_url == "https://ddagent-windows-stable.s3.amazonaws.com/" # Use a version of the Agent from the official repos
    dd_agent_installer_basename = "ddagent-cli-#{dd_agent_version}"
  else # Use a version of the Agent from the testing repos
    dd_agent_installer_basename = "datadog-agent-#{dd_agent_version}-1-x86_64"
  end
end

temp_file_basename = ::File.join(Chef::Config[:file_cache_path], 'ddagent-cli').gsub(File::SEPARATOR, File::ALT_SEPARATOR || File::SEPARATOR)

dd_agent_installer = "#{dd_agent_installer_basename}.msi"
source_url += dd_agent_installer
temp_file = "#{temp_file_basename}.msi"

log_file_name = ::File.join(Chef::Config[:file_cache_path], 'install.log').gsub(File::SEPARATOR, File::ALT_SEPARATOR || File::SEPARATOR)
# Delete the log file in case it exists (in case of multiple converge runs for example)
file log_file_name do
  action :delete
end

# Agent >= 5.12.0 installs per-machine by default, but specifying ALLUSERS=1 shouldn't affect the install
agent_install_options = node['dd-agent-install']['agent_install_options']
install_options = "/log #{log_file_name} /norestart ALLUSERS=1 #{agent_install_options}"

# This fake package resource serves only to trigger the Datadog Agent uninstall.
# If the checksum is not provided, assume we need to reinstall the Agent.
package 'Datadog Agent removal' do
  only_if { node['dd-agent-install']['windows_agent_checksum'].nil? }
  package_name 'Datadog Agent'
  action :remove
end

# When WIXFAILWHENDEFERRED is present, we expect the installer to fail.
expected_msi_result_code = [0, 3010]
expected_msi_result_code.append(1603) if agent_install_options.include?('WIXFAILWHENDEFERRED')

windows_package 'Datadog Agent' do
  source source_url
  checksum node['dd-agent-install']['windows_agent_checksum'] if node['dd-agent-install']['windows_agent_checksum']
  retries package_retries unless package_retries.nil?
  retry_delay package_retry_delay unless package_retry_delay.nil?
  options install_options
  action :install
  remote_file_attributes ({
    :path => temp_file
  })
  returns expected_msi_result_code
  # It's ok to ignore failure, the kitchen test will fail anyway
  # but we need to print the install logs.
  ignore_failure true
end

# This runs during the converge phase and will return a non-zero exit
# code if the service doesn't run. While it can be useful to quickly
# test locally if the Datadog Agent service is running after a converge phase
# it defeats the purpose of the kitchen tests, so keep it commented unless debugging the installer.
#execute "check-agent-service" do
#  command "sc interrogate datadogagent 2>&1"
#  action :run
#end

ruby_block "Print install logs" do
  only_if { ::File.exists?(log_file_name) }
  block do
    # Use warn, because Chef's default "log" is too chatty
    # and the kitchen tests default to "warn"
    Chef::Log.warn(File.open(log_file_name, "rb:UTF-16LE", &:read).encode('UTF-8'))
  end
end

ruby_block "Check install sucess" do
  not_if { agent_install_options.include?('WIXFAILWHENDEFERRED') }
  block do
    raise "Could not find installation log file, did the installer run ?" if !File.file?(log_file_name)
    logfile = File.open(log_file_name, "rb:UTF-16LE", &:read).encode('UTF-8')
    raise "The Agent failed to install" if logfile.include? "Product: Datadog Agent -- Installation failed."
  end
end
