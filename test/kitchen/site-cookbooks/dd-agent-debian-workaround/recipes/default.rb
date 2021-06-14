#
# Cookbook Name:: dd-agent-debian-workaround
# Recipe:: default
#
# Copyright (C) 2021-present Datadog
#
# All rights reserved - Do Not Redistribute
#

if node['platform_family'] == 'debian'
  package 'install-dirmngr' do
    package_name 'dirmngr'
    action :install
    ignore_failure true # Can fail on older distros where the dirmngr package does not exist, but shouldn't prevent install.
  end
end
