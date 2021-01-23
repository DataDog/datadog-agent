#
# Cookbook Name:: dd-security-agent-check
# Recipe:: stress-tests
#
# Copyright (C) 2020 Datadog
#

if node['platform_family'] != 'windows'
  wrk_dir = '/tmp/security-agent'

  directory wrk_dir do
    recursive true
  end

  cookbook_file "#{wrk_dir}/stresssuite" do
    source "stresssuite"
    mode '755'
  end

  cookbook_file "#{wrk_dir}/stresssuite-master" do
    source "stresssuite-master"
    mode '755'
  end

  ['polkit', 'unattended-upgrades', 'snapd', 'cron', 'walinuxagent',
   'multipathd', 'rsyslog', 'atd', 'chronyd', 'hv-kvp-daemon'].each do |s|
    service s do
        action :stop
    end
  end
end