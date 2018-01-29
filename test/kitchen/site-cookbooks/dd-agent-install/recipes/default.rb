#
# Cookbook Name:: dd-agent-install
# Recipe:: default
#
# Copyright (C) 2013 Datadog
#
# All rights reserved - Do Not Redistribute
#
require 'uri'

if node['platform_family'] == 'windows' && node['dd-agent-install']['agent6']
  include_recipe 'dd-agent-install::_install_windows'
else
  include_recipe 'datadog::dd-agent'
end
