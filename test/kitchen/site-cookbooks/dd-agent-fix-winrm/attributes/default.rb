#
# Cookbook Name:: dd-agent-fix-winrm
# Attributes:: default
#
# Copyright (C) 2023-present Datadog
#
# All rights reserved - Do Not Redistribute
#

default['dd-agent-fix-winrm']['enabled'] = false
# The amount here needs to match the MaxMemoryPerShellMB parameter set in driver_config
default['dd-agent-fix-winrm']['target_mb'] = "4096"
