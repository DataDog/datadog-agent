#
# Cookbook Name:: dd-agent-rhel-workaround
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#
# All rights reserved - Do Not Redistribute
#

if node['platform_family'] == 'rhel' && node['platform_version'].to_i >= 8
  execute 'increase / partition size' do
    command <<-EOF
      export size=32;
      export rootpart=$(cat /proc/mounts | awk '{ if ($2 =="/") print $1; }'); 
      while [[ $size -ne "0" ]] && ! sudo lvextend --size +${size}G ${rootpart}; do 
        size=$(($size / 2)); 
      done;
      sudo xfs_growfs /
    EOF
    live_stream true
  end
end
