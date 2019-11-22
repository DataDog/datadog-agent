#
# Cookbook Name:: dd-agent-step-by-step
# Recipe:: default
#
# Copyright (C) 2013 Datadog
#
# All rights reserved - Do Not Redistribute
#

case node['platform_family']
when 'debian'
  execute 'install dirmngr' do
    command <<-EOF
      sudo apt-get update
      cache_output=`apt-cache search dirmngr`
      if [ ! -z "$cache_output" ]; then
        sudo apt-get install -y dirmngr
      fi
    EOF
  end

  execute 'install debian' do
    command <<-EOF
      sudo sh -c "echo \'deb http://#{node['dd-agent-step-by-step']['repo_domain_apt']}/ #{node['dd-agent-step-by-step']['repo_branch_apt']} main\' > /etc/apt/sources.list.d/datadog.list"
      sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys A2923DFF56EDA6E76E55E492D3A80E30382E94DE
      sudo apt-get update
      sudo apt-get install #{node['dd-agent-step-by-step']['package_name']} -y -q
    EOF
  end

when 'rhel'
  protocol = node['platform_version'].to_i < 6 ? 'http' : 'https'

  file '/etc/yum.repos.d/datadog.repo' do
    content <<-EOF.gsub(/^ {6}/, '')
      [datadog]
      name = Datadog, Inc.
      baseurl = #{node['dd-agent-step-by-step']['yumrepo']}
      enabled=1
      gpgcheck=1
      repo_gpgcheck=0
      gpgkey=#{protocol}://yum.datadoghq.com/DATADOG_RPM_KEY.public
             #{protocol}://yum.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public
    EOF
  end

  execute 'install rhel' do
    command <<-EOF
      sudo yum makecache
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
      repo_gpgcheck=0
      gpgkey=https://yum.datadoghq.com/DATADOG_RPM_KEY.public
             https://yum.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public
    EOF
  end

  execute 'install suse' do
    command <<-EOF
      sudo rpm --import https://yum.datadoghq.com/DATADOG_RPM_KEY.public
      sudo rpm --import https://yum.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public
      sudo zypper --non-interactive refresh datadog
      sudo zypper --non-interactive install #{node['dd-agent-step-by-step']['package_name']}
    EOF
  end
end

if node['platform_family'] != 'windows'
  execute 'add config file' do
    command <<-EOF
      sudo sh -c "sed \'s/api_key:.*/api_key: #{node['dd-agent-step-by-step']['api_key']}/\' \
      /etc/datadog-agent/datadog.yaml.example > /etc/datadog-agent/datadog.yaml"
    EOF
  end
end

if node['platform_family'] == 'windows'
end

service_provider = nil
if (((node['platform'] == 'amazon' || node['platform_family'] == 'amazon') && node['platform_version'].to_i != 2) ||
    (node['platform'] == 'ubuntu' && node['platform_version'].to_f < 15.04) || # chef <11.14 doesn't use the correct service provider
   (node['platform'] != 'amazon' && node['platform_family'] == 'rhel' && node['platform_version'].to_i < 7))
  # use Upstart provider explicitly for Agent 6 on Amazon Linux < 2.0 and RHEL < 7
  service_provider = Chef::Provider::Service::Upstart
end

service 'datadog-agent' do
  provider service_provider unless service_provider.nil?
  action :start
end
