#
# Cookbook Name:: dd-system-probe-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#

rootdir = value_for_platform(
  'windows' => { 'default' => ::File.join(Chef::Config[:file_cache_path], 'system-probe') },
  'default' => '/tmp/ci/system-probe'
)

directory rootdir do
  recursive true
end

# This will copy the whole file tree from COOKBOOK_NAME/files/default/tests
# to the directory where RSpec is expecting them.
remote_directory ::File.join(rootdir, "tests") do
  source 'tests'
  mode '755'
  files_mode '755'
  sensitive true
  unless platform?('windows')
    files_owner 'root'
  end
end

if platform?('windows')
  include_recipe "::windows"
else
  include_recipe "::linux"
end
