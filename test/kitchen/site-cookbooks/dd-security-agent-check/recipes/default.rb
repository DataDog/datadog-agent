#
# Cookbook Name:: dd-security-agent-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#

directory "#{node['common']['work_dir']}/tests" do
  recursive true
end

include_recipe "::functional-tests"

if node['platform_family'] != 'windows'
  include_recipe "::stress-tests"
end
