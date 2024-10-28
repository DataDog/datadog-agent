#
# Cookbook Name:: dd-agent-install
# Recipe:: _repair_windows_install
#
# Copyright (C) 2021-present Datadog

powershell_script "repair-agent" do
  code <<-EOF
  $product_code = (Get-WmiObject Win32_Product | Where-Object -Property Name -eq 'Datadog Agent').IdentifyingNumber
  Start-Process msiexec.exe -Wait -ArgumentList '/q','/log','repair.log','/fa',$product_code
  EOF
end
