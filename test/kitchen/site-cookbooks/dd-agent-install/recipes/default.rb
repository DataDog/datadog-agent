#
# Cookbook Name:: dd-agent-install
# Recipe:: default
#
# Copyright (C) 2013 Datadog
#
# All rights reserved - Do Not Redistribute
#
require 'uri'

if node['platform_family'] == 'windows' && node['dd-agent-install']['agent_major_version'].to_i > 5
  include_recipe 'dd-agent-install::_install_windows'
else
  if node['platform_family'] == 'debian'
    package 'install-dirmngr' do
      package_name 'dirmngr'
      action :install
      ignore_failure true # Can fail on older distros where the dirmngr package does not exist, but shouldn't prevent install.
    end
  end

  include_recipe 'datadog::dd-agent'
end
