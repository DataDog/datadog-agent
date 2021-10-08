#
# Cookbook Name:: dd-agent-install
# Recipe:: default
#
# Copyright (C) 2013-present Datadog
#
# All rights reserved - Do Not Redistribute
#
require 'uri'

if node['platform_family'] == 'windows' 
  include_recipe 'dd-agent-5::_install_windows_base'
else
  include_recipe 'dd-agent-5::_install_linux'
end
