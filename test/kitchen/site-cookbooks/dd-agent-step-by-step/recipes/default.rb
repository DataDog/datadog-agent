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
  execute 'install debian' do
    command <<-EOF
      sudo sh -c "echo \'deb http://#{node['dd-agent-step-by-step']['repo_domain_apt']}/ #{node['dd-agent-step-by-step']['repo_branch_apt']} main\' > /etc/apt/sources.list.d/datadog.list"
      sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 382E94DE
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
    EOF
  end

  execute 'install suse' do
    command <<-EOF
      sudo rpm --import https://yum.datadoghq.com/DATADOG_RPM_KEY.public
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

service 'datadog-agent' do
  action :start
end
