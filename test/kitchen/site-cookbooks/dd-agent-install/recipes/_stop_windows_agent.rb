#
# Cookbook Name:: dd-agent-install
# Recipe:: _stop_windows_agent
#
# Copyright (C) 2021-present Datadog

powershell_script 'stop-datadog-agent' do
  code <<-EOH
    $serviceName = "#{node['dd-agent-install']['agent_name']}"
    sc.exe query $serviceName;
    $agentService = Get-Service -Name $serviceName;
    if ($agentService.Status -eq "running")
    {
      foreach($dependentService in $agentService.DependentServices | Where-Object { $_.status -eq 'running' })
      {
        Write-Host "Stopping " + $dependentService.name;
        sc.exe stop $dependentService.name
      }
      sc.exe stop $serviceName
    }
  EOH
end
