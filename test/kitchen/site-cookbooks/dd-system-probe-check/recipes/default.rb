#
# Cookbook Name:: dd-system-probe-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#

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
  case
  when !platform?('windows')
    files_owner 'root'
  end
end

if platform?('windows')
  include_recipe "::windows"
else
  include_recipe "::linux"
end
