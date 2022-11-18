#
# Cookbook Name:: dd-system-probe-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#

if !platform?('windows')
  include_recipe "::linux_use_azure_mnt"
end

# This will copy the whole file tree from COOKBOOK_NAME/files/default/tests
# to the directory where RSpec is expecting them.
testdir = value_for_platform(
  'windows' => { 'default' => ::File.join(Chef::Config[:file_cache_path], 'system-probe-tests') },
  'default' => '/tmp/system-probe-tests'
)

remote_directory testdir do
  source 'tests'
  mode '755'
  files_mode '755'
  sensitive true
  unless platform?('windows')
    files_owner 'root'
  end
end

file ::File.join(testdir, 'color_idx') do
  content node[:color_idx].to_s
  unless platform?('windows')
    mode 644
  end
end

if platform?('windows')
  include_recipe "::windows"
else
  include_recipe "::linux"
end
