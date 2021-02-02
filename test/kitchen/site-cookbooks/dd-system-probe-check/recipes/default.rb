#
# Cookbook Name:: dd-system-probe-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#
if platform?('centos')
  include_recipe 'yum-centos::vault'
end

kernel_version = `uname -r`.strip
package 'kernel headers' do
  case node[:platform]
  when 'redhat', 'centos', 'fedora'
    package_name "kernel-devel-#{kernel_version}"
  when 'ubuntu', 'debian'
    package_name "linux-headers-#{kernel_version}"
  end
end

package 'conntrack'

package 'netcat' do
  case node[:platform]
  when 'redhat', 'centos', 'fedora'
    package_name 'nc'
  else
    package_name 'netcat'
  end
end

package 'socat'

# Enable IPv6 support
kernel_module 'ipv6' do
  action :load
end
execute 'sysctl net.ipv6.conf.all.disable_ipv6=0'

# This will copy the whole file tree from COOKBOOK_NAME/files/default/tests
# to the directory /tmp/system-probe-tests where RSpec is expecting them.
remote_directory "/tmp/system-probe-tests" do
  source 'tests'
  mode 755
end

# The remote_directory resource doesn't inherit the permissions (inherit and
# mode options don't work) so we make the test files executable
execute 'chmod test files' do
  command "chmod -R 755 /tmp/system-probe-tests"
  user "root"
  action :run
end
