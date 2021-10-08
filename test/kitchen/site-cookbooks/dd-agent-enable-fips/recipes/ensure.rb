#
# Cookbook Name:: dd-agent-enable-fips
# Recipe:: ensure
#
# Copyright (C) 2021-present Datadog
#
# All rights reserved - Do Not Redistribute
#

if ['redhat', 'centos', 'fedora'].include?(node[:platform])
  execute 'enable FIPS mode' do
    command 'fips-mode-setup --is-enabled'
  end
end
