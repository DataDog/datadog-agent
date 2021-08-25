#
# Cookbook Name:: dd-agent-install
# Recipe:: _stop_windows_agent
#
# Copyright (C) 2021-present Datadog

windows_service 'datadog-agent' do
  service_name node['dd-agent-install']['agent_name']
  action :stop
end
