#
# Cookbook Name:: dd-agent-sles-workaround
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#
# All rights reserved - Do Not Redistribute
#

if node['platform_family'] == 'suse' && node.key?('azure')
  # Update the waagent conf to stop watching hostname changes.
  execute 'update Azure Agent conf' do
    command "sed -i 's/Provisioning\\.MonitorHostName=y/Provisioning\\.MonitorHostName=n/' /etc/waagent.conf"
  end

  # Change the Windows Azure Agent conf to stop watching hostname changes.
  # For some reason it's changing the hostname every 30s on SLES 12, which
  # leads to a network interface reset, making it likely for tests to fail if a
  # network call happens during the reset.
  service 'waagent' do
    service_name "waagent"
    action :restart
  end

  # Put eth0 back up in case the waagent was taking it down while we restarted it.
  execute 'bring eth0 up' do
    command "/sbin/ifup eth0"
  end
end
