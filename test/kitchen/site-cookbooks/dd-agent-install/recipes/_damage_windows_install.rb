#
# Cookbook Name:: dd-agent-install
# Recipe:: _damage_windows_install
#
# Copyright (C) 2021-present Datadog

powershell_script "damage-agent" do
  code <<-EOF
  Remove-Item -Recurse -Force 'C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe'
  EOF
end
