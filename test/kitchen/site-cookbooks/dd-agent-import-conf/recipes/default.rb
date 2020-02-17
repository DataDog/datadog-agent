# Cookbook Name:: dd-agent-imprt-conf
# Recipe:: default
#
# Copyright (C) 2013 Datadog
#
# All rights reserved - Do Not Redistribute
#

# We only do the import, meaning that the Agent 5 and
# the Agent 6 has already been setup.
# We just have to run the command.


if node['platform_family'] != 'windows'
  include_recipe 'dd-agent-import-conf::_datadog-agent-6'

  execute "import-command" do
    command "sudo -u dd-agent -- /opt/datadog-agent/bin/agent/agent import /etc/dd-agent/ /etc/datadog-agent/"
    action :run
    notifies :restart, 'service[datadog-agent-6]'
  end
end

if node['platform_family'] == 'windows'
  # TODO(remy): do we want to test the import on Windows?
end
