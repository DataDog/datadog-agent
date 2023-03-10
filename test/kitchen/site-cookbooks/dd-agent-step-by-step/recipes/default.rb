#
# Cookbook Name:: dd-agent-step-by-step
# Recipe:: default
#
# Copyright (C) 2013-present Datadog
#
# All rights reserved - Do Not Redistribute
#

case node['platform_family']
when 'debian'
  apt_trusted_d_keyring='/etc/apt/trusted.gpg.d/datadog-archive-keyring.gpg'
  apt_usr_share_keyring='/usr/share/keyrings/datadog-archive-keyring.gpg'

  execute 'create /usr/share keyring and source list' do
    command <<-EOF
      sudo apt-get install -y apt-transport-https curl gnupg
      sudo sh -c "echo \'deb #{node['dd-agent-step-by-step']['aptrepo']} #{node['dd-agent-step-by-step']['aptrepo_dist']} #{node['dd-agent-step-by-step']['agent_major_version']}\' > /etc/apt/sources.list.d/datadog.list"
      sudo touch #{apt_usr_share_keyring} && sudo chmod a+r #{apt_usr_share_keyring}
      for key in DATADOG_APT_KEY_CURRENT.public DATADOG_APT_KEY_F14F620E.public DATADOG_APT_KEY_382E94DE.public; do
        sudo curl --retry 5 -o "/tmp/${key}" "https://keys.datadoghq.com/${key}"
        sudo cat "/tmp/${key}" | sudo gpg --import --batch --no-default-keyring --keyring "#{apt_usr_share_keyring}"
      done
    EOF
  end

  execute 'create /etc/apt keyring' do
    only_if { (platform?('ubuntu') && node['platform_version'].to_i < 16) || (platform?('debian') && node['platform_version'].to_i < 9) }
    command <<-EOF
      sudo cp #{apt_usr_share_keyring} #{apt_trusted_d_keyring}
    EOF
  end

  execute 'install debian' do
    command <<-EOF
      sudo apt-get update
      sudo apt-get install #{node['dd-agent-step-by-step']['package_name']} -y -q
    EOF
  end

when 'rhel'
  if platform?('centos') && node['dd-agent-rspec'] && node['dd-agent-rspec']['enable_cws']
    # TODO(lebauce): enable repositories to install package
    # package 'policycoreutils-python'

    execute 'set SElinux to permissive mode to be able to start system-probe' do
      command "setenforce 0"
    end
  end

  protocol = node['platform_version'].to_i < 6 ? 'http' : 'https'
  # Because of https://bugzilla.redhat.com/show_bug.cgi?id=1792506, we disable
  # repo_gpgcheck on RHEL/CentOS < 8.2
  repo_gpgcheck = node['platform_version'].to_f < 8.2 ? '0' : '1'

  file '/etc/yum.repos.d/datadog.repo' do
    content <<-EOF.gsub(/^ {6}/, '')
      [datadog]
      name = Datadog, Inc.
      baseurl = #{node['dd-agent-step-by-step']['yumrepo']}
      enabled=1
      gpgcheck=1
      repo_gpgcheck=#{repo_gpgcheck}
      gpgkey=#{protocol}://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public
             #{protocol}://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public
             #{protocol}://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public
    EOF
  end

  execute 'install rhel' do
    command <<-EOF
      sudo yum makecache -y
      sudo yum install -y #{node['dd-agent-step-by-step']['package_name']}
    EOF
  end

when 'suse'
  file '/etc/zypp/repos.d/datadog.repo' do
    content <<-EOF.gsub(/^ {6}/, '')
      [datadog]
      name = Datadog, Inc.
      baseurl = #{node['dd-agent-step-by-step']['yumrepo_suse']}
      enabled=1
      gpgcheck=1
      repo_gpgcheck=1
      gpgkey=https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public
             https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public
             https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public
    EOF
  end

  execute 'install suse' do
    command <<-EOF
      sudo curl -o /tmp/DATADOG_RPM_KEY_CURRENT.public https://keys.datadoghq.com/DATADOG_RPM_KEY_CURRENT.public
      sudo rpm --import /tmp/DATADOG_RPM_KEY_CURRENT.public
      sudo curl -o /tmp/DATADOG_RPM_KEY_FD4BF915.public https://keys.datadoghq.com/DATADOG_RPM_KEY_FD4BF915.public
      sudo rpm --import /tmp/DATADOG_RPM_KEY_FD4BF915.public
      sudo curl -o /tmp/DATADOG_RPM_KEY_E09422B3.public https://keys.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public
      sudo rpm --import /tmp/DATADOG_RPM_KEY_E09422B3.public
      sudo zypper --non-interactive --no-gpg-checks refresh datadog
      sudo zypper --non-interactive install #{node['dd-agent-step-by-step']['package_name']}
    EOF
  end
end

if node['platform_family'] != 'windows'
  execute 'add config file' do
    command <<-EOF
      sudo sh -c "sed \'s/api_key:.*/api_key: #{node['dd-agent-step-by-step']['api_key']}/\' \
      /etc/datadog-agent/datadog.yaml.example > /etc/datadog-agent/datadog.yaml"
      sudo sh -c "chown dd-agent:dd-agent /etc/datadog-agent/datadog.yaml && chmod 640 /etc/datadog-agent/datadog.yaml"
    EOF
  end
end

if node['platform_family'] == 'windows'
end

service_provider = nil
if node['dd-agent-step-by-step']['agent_major_version'].to_i > 5 &&
  (((node['platform'] == 'amazon' || node['platform_family'] == 'amazon') && node['platform_version'].to_i != 2) ||
  (node['platform'] == 'ubuntu' && node['platform_version'].to_f < 15.04) || # chef <11.14 doesn't use the correct service provider
  (node['platform'] != 'amazon' && node['platform_family'] == 'rhel' && node['platform_version'].to_i < 7))
  # use Upstart provider explicitly for Agent 6 on Amazon Linux < 2.0 and RHEL < 7
  service_provider = Chef::Provider::Service::Upstart
end

service 'datadog-agent' do
  provider service_provider unless service_provider.nil?
  action :start
end
