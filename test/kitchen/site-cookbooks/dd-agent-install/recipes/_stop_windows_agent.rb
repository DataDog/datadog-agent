#
# Cookbook Name:: dd-agent-install
# Recipe:: _stop_windows_agent
#
# Copyright (C) 2021-present Datadog

powershell_script 'stop-datadog-agent' do
  code <<-EOH
    Stop-Service -Force -Name "#{node['dd-agent-install']['agent_name']}"
  EOH
end
