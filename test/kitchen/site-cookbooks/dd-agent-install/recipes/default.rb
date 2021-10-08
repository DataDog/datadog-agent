#
# Cookbook Name:: dd-agent-install
# Recipe:: default
#
# Copyright (C) 2013-present Datadog
#
# All rights reserved - Do Not Redistribute
#
require 'uri'

if node['platform_family'] == 'windows' && node['dd-agent-install']['agent_major_version'].to_i > 5
  include_recipe 'dd-agent-install::_install_windows'
else
  include_recipe 'datadog::dd-agent'
end
