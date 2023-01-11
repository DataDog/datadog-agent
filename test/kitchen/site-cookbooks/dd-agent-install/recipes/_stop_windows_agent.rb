#
# Cookbook Name:: dd-agent-install
# Recipe:: _stop_windows_agent
#
# Copyright (C) 2021-present Datadog

powershell_script 'stop-datadog-agent' do
  code <<-EOH
    $serviceName = "#{node['dd-agent-install']['agent_name']}"
    sc.exe query $serviceName;
    $agentService = Get-Service -Name $serviceName
    if ($agentService.Status -eq "running")
    {
      Get-Service -CN .
      | Where-Object { $_.status -eq 'running' -and $_.Name -eq "#{node['dd-agent-install']['agent_name']}" }
      | ForEach-Object
      {
        foreach($s in $_.DependentServices | Where-Object { $_.status -eq 'running' })
        {
          Write-Host "Stopping " + $s.name;
          sc.exe stop $s.name
        }
      }
      sc.exe stop $serviceName
    }
  EOH
end
