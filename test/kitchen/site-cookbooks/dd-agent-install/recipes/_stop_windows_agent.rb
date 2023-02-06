#
# Cookbook Name:: dd-agent-install
# Recipe:: _stop_windows_agent
#
# Copyright (C) 2021-present Datadog

powershell_script 'stop-datadog-agent' do
  code <<-EOH
    $serviceName = "#{node['dd-agent-install']['agent_name']}"
    sc.exe query $serviceName;
    Stop-Service -force $serviceName
  EOH
end
