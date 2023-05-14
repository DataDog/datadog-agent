#
# Cookbook Name:: dd-security-agent-check
# Recipe:: stress-tests
#
# Copyright (C) 2020-present Datadog
#

if node['platform_family'] != 'windows'
  cookbook_file "#{node['common']['work_dir']}/tests/stresssuite" do
    source "tests/stresssuite"
    mode '755'
  end

  ['polkit', 'unattended-upgrades', 'snapd', 'cron', 'walinuxagent',
   'multipathd', 'rsyslog', 'atd', 'chronyd', 'hv-kvp-daemon'].each do |s|
    service s do
        action :stop
    end
  end
end
