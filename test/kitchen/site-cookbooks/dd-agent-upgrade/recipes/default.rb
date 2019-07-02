#
# Cookbook Name:: dd-agent-upgrade
# Recipe:: default
#
# Copyright (C) 2013 Datadog
#
# All rights reserved - Do Not Redistribute
#
require 'uri'

if node['dd-agent-upgrade']['add_new_repo']
  case node['platform_family']
  when 'debian'
    include_recipe 'apt'

    apt_repository 'datadog-update' do
      keyserver 'keyserver.ubuntu.com'
      key 'A2923DFF56EDA6E76E55E492D3A80E30382E94DE'
      uri node['dd-agent-upgrade']['aptrepo']
      distribution node['dd-agent-upgrade']['aptrepo_dist']
      components ['main']
      action :add
    end

  when 'rhel'
    include_recipe 'yum'

    yum_repository 'datadog-update' do
      name 'datadog-update'
      description 'datadog-update'
      url node['dd-agent-upgrade']['yumrepo']
      action :add
      make_cache true
      # Older versions of yum embed M2Crypto with SSL that doesn't support TLS1.2
      protocol = node['platform_version'].to_i < 6 ? 'http' : 'https'
      gpgkey "#{protocol}://yum.datadoghq.com/DATADOG_RPM_KEY.public"
    end
  when 'suse'
    old_key_local_path = ::File.join(Chef::Config[:file_cache_path], 'DATADOG_RPM_KEY.public')
    remote_file 'DATADOG_RPM_KEY.public' do
      path old_key_local_path
      source node['datadog']['yumrepo_gpgkey']
      # not_if 'rpm -q gpg-pubkey-4172a230' # (key already imported)
      notifies :run, 'execute[rpm-import datadog key 4172a230]', :immediately
    end

    # Import key if fingerprint matches
    execute 'rpm-import datadog key 4172a230' do
      command "rpm --import #{old_key_local_path}"
      only_if "gpg --dry-run --quiet --with-fingerprint #{old_key_local_path} | grep '60A3 89A4 4A0C 32BA E3C0  3F0B 069B 56F5 4172 A230'"
      action :nothing
    end

    zypper_repository 'datadog-update' do
      name 'datadog-update'
      description 'datadog-update'
      baseurl node['dd-agent-upgrade']['yumrepo_suse']
      action :add
      gpgcheck false
      # Older versions of yum embed M2Crypto with SSL that doesn't support TLS1.2
      protocol = node['platform_version'].to_i < 6 ? 'http' : 'https'
      gpgkey "#{protocol}://yum.datadoghq.com/DATADOG_RPM_KEY.public"
    end
  end
end

if node['platform_family'] != 'windows'
  package node['dd-agent-upgrade']['package_name'] do
    action :upgrade
    version node['dd-agent-upgrade']['version']
  end
  # the :upgrade method seems broken for sles: https://github.com/chef/chef/issues/4863
  if node['platform_family'] == 'suse'
    package node['dd-agent-upgrade']['package_name'] do
      action :remove
    end
    package node['dd-agent-upgrade']['package_name'] do
      action :install
      version node['dd-agent-upgrade']['version']
    end
  end
end

if node['platform_family'] == 'windows'
  package_retries = node['dd-agent-upgrade']['agent_package_retries']
  package_retry_delay = node['dd-agent-upgrade']['agent_package_retry_delay']
  dd_agent_version = node['dd-agent-upgrade']['windows_version']
  dd_agent_filename = node['dd-agent-upgrade']['windows_agent_filename']

  if dd_agent_filename
    dd_agent_installer_basename = dd_agent_filename
  else
    if dd_agent_version
      dd_agent_installer_basename = "datadog-agent-#{dd_agent_version}-1-x86_64"
    else
      dd_agent_installer_basename = "datadog-agent-6.0.0-beta.latest.amd64"
    end
  end

  temp_file_basename = ::File.join(Chef::Config[:file_cache_path], 'ddagent-up').gsub(File::SEPARATOR, File::ALT_SEPARATOR || File::SEPARATOR)

  dd_agent_installer = "#{dd_agent_installer_basename}.msi"
  temp_file = "#{temp_file_basename}.msi"
  installer_type = :msi
  # Agent >= 5.12.0 installs per-machine by default, but specifying ALLUSERS=1 shouldn't affect the install
  install_options = '/norestart ALLUSERS=1'
  use_windows_package_resource = true

  source_url = node['dd-agent-upgrade']['windows_agent_url']
  if !source_url.end_with? '/'
    source_url += '/'
  end
  source_url += dd_agent_installer

    # Download the installer to a temp location
  remote_file temp_file do
    source source_url
    checksum node['dd-agent-upgrade']['windows_agent_checksum'] if node['dd-agent-upgrade']['windows_agent_checksum']
    retries package_retries unless package_retries.nil?
    retry_delay package_retry_delay unless package_retry_delay.nil?
  end

  execute "install-agent" do
    command "start /wait msiexec /log upgrade.log /q /i #{temp_file} #{install_options}"
    action :run
    notifies :restart, 'service[datadog-agent]'
  end

end
