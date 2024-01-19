#
# Cookbook Name:: dd-security-agent-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#
directory "#{node['common']['work_dir']}/tests" do
  recursive true
end
  