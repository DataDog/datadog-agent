#
# Cookbook Name:: dd-agent-install
# Recipe:: _stop_windows_agent
#
# Copyright (C) 2021-present Datadog

powershell_script 'stop-datadog-agent' do
  code <<-EOH
    sc.exe query "#{node['dd-agent-install']['agent_name']}"
    sc.exe stop "#{node['dd-agent-install']['agent_name']}"
  EOH
end
