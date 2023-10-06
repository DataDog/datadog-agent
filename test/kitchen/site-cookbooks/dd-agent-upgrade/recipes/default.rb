#
# Cookbook Name:: dd-agent-upgrade
# Recipe:: default
#
# Copyright (C) 2013-present Datadog
#
# All rights reserved - Do Not Redistribute
#
require 'uri'

if node['dd-agent-upgrade']['add_new_repo']
  case node['platform_family']
  when 'debian'
    include_recipe 'apt'
    apt_trusted_d_keyring = '/etc/apt/trusted.gpg.d/datadog-archive-keyring.gpg'
    apt_usr_share_keyring = '/usr/share/keyrings/datadog-archive-keyring.gpg'
    apt_gpg_keys = {
      'DATADOG_APT_KEY_CURRENT.public'           => 'https://keys.datadoghq.com/DATADOG_APT_KEY_CURRENT.public',
      'D75CEA17048B9ACBF186794B32637D44F14F620E' => 'https://keys.datadoghq.com/DATADOG_APT_KEY_F14F620E.public',
      'A2923DFF56EDA6E76E55E492D3A80E30382E94DE' => 'https://keys.datadoghq.com/DATADOG_APT_KEY_382E94DE.public',
    }

    package 'install dependencies' do
      package_name ['apt-transport-https', 'gnupg']
      action :install
    end

    file apt_usr_share_keyring do
      action :create_if_missing
      content ''
      mode '0644'
    end

    apt_gpg_keys.each do |key_fingerprint, key_url|
      # Download the APT key
      key_local_path = ::File.join(Chef::Config[:file_cache_path], key_fingerprint)
      # By default, remote_file will use `If-Modified-Since` header to see if the file
      # was modified remotely, so this works fine for the "current" key
      remote_file "remote_file_#{key_fingerprint}" do
        path key_local_path
        source key_url
        notifies :run, "execute[import apt datadog key #{key_fingerprint}]", :immediately
      end

      # Import the APT key
      execute "import apt datadog key #{key_fingerprint}" do
        command "/bin/cat #{key_local_path} | gpg --import --batch --no-default-keyring --keyring #{apt_usr_share_keyring}"
        # the second part extracts the fingerprint of the key from output like "fpr::::A2923DFF56EDA6E76E55E492D3A80E30382E94DE:"
        not_if "/usr/bin/gpg --no-default-keyring --keyring #{apt_usr_share_keyring} --list-keys --with-fingerprint --with-colons | grep \
               $(cat #{key_local_path} | gpg --with-colons --with-fingerprint 2>/dev/null | grep 'fpr:' | sed 's|^fpr||' | tr -d ':')"
        action :nothing
      end
    end

    remote_file apt_trusted_d_keyring do
      action :create
      mode '0644'
      source "file://#{apt_usr_share_keyring}"
      only_if { (platform?('ubuntu') && node['platform_version'].to_i < 16) || (platform?('debian') && node['platform_version'].to_i < 9) }
    end

    # Add APT repositories
    # Chef's apt_repository resource doesn't allow specifying the signed-by option and we can't pass
    # it in uri, as that would make it fail parsing, hence we use the file and apt_update resources.
    apt_update 'datadog' do
      retries retries
      ignore_failure true # this is exactly what apt_repository does
      action :nothing
    end

    file '/etc/apt/sources.list.d/datadog.list' do
      action :create
      owner 'root'
      group 'root'
      mode '0644'
      content "deb #{node['dd-agent-upgrade']['aptrepo']} #{node['dd-agent-upgrade']['aptrepo_dist']} #{node['dd-agent-upgrade']['agent_major_version']}"
      notifies :update, 'apt_update[datadog]', :immediately
    end

  when 'rhel'
    include_recipe 'yum'

    yum_repository 'datadog' do
      name 'datadog'
      description 'datadog'
      url node['dd-agent-upgrade']['yumrepo']
      action :add
      make_cache true
      # Older versions of yum embed M2Crypto with SSL that doesn't support TLS1.2
      protocol = node['platform_version'].to_i < 6 ? 'http' : 'https'
      gpgkey [
        "#{protocol}://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public",
        "#{protocol}://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public",
        "#{protocol}://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public",
      ]
    end
  when 'suse'
    zypper_repository 'datadog' do
      name 'datadog'
      description 'datadog'
      baseurl node['dd-agent-upgrade']['yumrepo_suse']
      action :add
      gpgcheck false
      # Older versions of yum embed M2Crypto with SSL that doesn't support TLS1.2
      protocol = node['platform_version'].to_i < 6 ? 'http' : 'https'
      gpgkey "#{protocol}://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public"
      gpgautoimportkeys false
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
    # We have this commented and run it as `execute` command to be able to provide
    # ZYPP_RPM_DEBUG=1 and see debug output. Whenever we solve/understand
    # https://bugzilla.suse.com/show_bug.cgi?id=1192034, we can uncomment
    # and remove the command.
    #
    # package node['dd-agent-upgrade']['package_name'] do
    #   action :install
    #   version node['dd-agent-upgrade']['version']
    # end
    execute 'install agent' do
      command "zypper --non-interactive install --auto-agree-with-licenses #{node['dd-agent-upgrade']['package_name']}=#{node['dd-agent-upgrade']['version']}"

      environment({'ZYPP_RPM_DEBUG' => '1'})
      live_stream true
      action :run
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
  agent_install_options = node['dd-agent-upgrade']['agent_install_options']
  install_options = "/norestart ALLUSERS=1  #{agent_install_options}"

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
    # notifies :restart, 'service[datadog-agent]'
  end

end
