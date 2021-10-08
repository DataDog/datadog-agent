#
# Cookbook Name:: dd-agent-enable-fips
# Recipe:: enable
#
# Copyright (C) 2021-present Datadog
#
# All rights reserved - Do Not Redistribute
#

if ['redhat', 'centos', 'fedora'].include?(node[:platform])
  execute 'enable FIPS mode' do
    command 'fips-mode-setup --enable'
    not_if 'fips-mode-setup --is-enabled'
  end

  # We need to set PubkeyAcceptedTypes to use ssh-rsa otherwise kitchen (which uses net-ssh)
  # won't be able to connect via ssh (it's disabled by default in FIPS mode).
  # https://docs.microsoft.com/en-us/cpp/linux/set-up-fips-compliant-secure-remote-linux-development
  # https://github.com/net-ssh/net-ssh/issues/712#issuecomment-628188633
  file '/etc/crypto-policies/back-ends/opensshserver.config' do
    action :create
    owner 'root'
    group 'root'
    mode '0644'
    content File.read("/etc/crypto-policies/back-ends/opensshserver.config").gsub(/-oPubkeyAcceptedKeyTypes=/, '-oPubkeyAcceptedKeyTypes=ssh-rsa,')
    not_if 'fips-mode-setup --is-enabled'
    notifies :reboot_now, 'reboot[fips]', :immediately
  end

  reboot 'fips' do
    not_if 'fips-mode-setup --is-enabled'
    reason 'Rebooting to boot up with FIPS mode enabled'
  end
end
