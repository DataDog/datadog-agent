#
# Cookbook Name:: dd-agent-fix-winrm
# Recipe:: default
#
# Copyright (C) 2023-present Datadog
#
# All rights reserved - Do Not Redistribute
#
if node['dd-agent-fix-winrm']['enabled']
  require "chef/mixin/powershell_out"
  ::Chef::Recipe.send(:include, Chef::Mixin::PowershellOut)

  winrm_memory = powershell_out!('(Get-Item WSMan:\localhost\Plugin\Microsoft.PowerShell\Quotas\MaxMemoryPerShellMB).value').stdout.chomp
  if winrm_memory != node['dd-agent-fix-winrm']['target_mb']
    # The amount here needs to match the MaxMemoryPerShellMB parameter set in driver_config
    powershell_script 'Update WinRM Powershell memory settings' do
      code "Set-Item WSMan:\\localhost\\Plugin\\Microsoft.PowerShell\\Quotas\\MaxMemoryPerShellMB #{node['dd-agent-fix-winrm']['target_mb']}"
      notifies :reboot_now, 'reboot[WinRM]', :immediately
    end

    reboot 'WinRM' do
      reason 'Rebooting to reload WinRM service settings'
    end
  end
end
