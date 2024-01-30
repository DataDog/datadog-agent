#
# Cookbook Name:: dd-security-agent-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#


if platform?('windows')
  include_recipe "::windows"
else
  include_recipe "::linux"
end
