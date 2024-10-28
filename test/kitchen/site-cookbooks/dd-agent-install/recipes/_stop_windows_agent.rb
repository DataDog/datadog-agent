#
# Cookbook Name:: dd-agent-install
# Recipe:: _stop_windows_agent
#
# Copyright (C) 2021-present Datadog

#
# gives the agent 60s to actually get started if it hasn't fully started yet
#
powershell_script 'stop-datadog-agent' do
  code <<-EOH
    $serviceName = "#{node['dd-agent-install']['agent_name']}"
    sc.exe query $serviceName;
    for($i= 0; $i -lt 30; $i++) {
      $s = (get-service datadogagent).status
      if($s -ne "StartPending") {
        break;
      }
      start-sleep -seconds 2
    }
    Stop-Service -force $serviceName
  EOH
end
