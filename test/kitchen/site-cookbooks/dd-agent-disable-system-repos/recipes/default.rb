#
# Cookbook Name:: dd-agent-disable-system-repos
# Recipe:: default
#
# Copyright (C) 2021-present Datadog
#
# All rights reserved - Do Not Redistribute
#

if ['redhat', 'centos', 'fedora'].include?(node[:platform])
  execute 'disable all yum repositories' do
    command 'yum-config-manager --disable "*"'
  end
elsif ['suse', 'opensuseleap'].include?(node[:platform])
  execute 'disable all zypper repositories' do
    # Zypper appears to be running when the VM starts, so we
    # wait for up to 2 minutes for it to release its lock
    command 'ZYPP_LOCK_TIMEOUT=120 zypper mr -da'
  end
end
