#
# Cookbook Name:: dd-system-probe-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#

if !platform?('windows')
  include_recipe "::linux_use_azure_mnt"
end

# grow root fs if needed
if !platform?('windows')
  package 'growpart' do
    case node[:platform]
    when 'amazon', 'redhat', 'centos', 'fedora'
      package_name 'cloud-utils-growpart'
    else
      package_name 'cloud-guest-utils'
    end
  end

  package 'gdisk'

  execute 'increase space' do
    command <<-EOF
      df -Th /

      d=$(df -T --block-size=1M / | tail -n1)
      dev_name=$(echo $d | awk '{print $1}')
      fstype=$(echo $d | awk '{print $2}')
      avail=$(echo $d | awk '{print $5}')
      mnt=$(echo $d | awk '{print $7}')

      if [[ $(awk "BEGIN {print 10000.0<$avail}") -eq 1 ]]; then
         echo "skip because use $avail > 10G"
         exit 0
      fi

      if [[ ${dev_name} =~ ^/dev/mapper ]]; then
         lvresize -L +10G ${dev_name}
         if [[ ${fstype} = "xfs" ]]; then
            xfs_growfs -d ${mnt}
         else
            resize2fs ${dev_name}
         fi
      fi
      if [[ ${dev_name} =~ ^/dev/nvme ]]; then
         disk=$(echo $dev_name | awk -Fp '{print $1}')
         partnum=$(echo $dev_name | awk -Fp '{print $2}')

         growpart ${disk} ${partnum}
         if [[ ${fstype} = "xfs" ]]; then
            xfs_growfs -d ${mnt}
         else
            resize2fs ${dev_name}
         fi
      fi

      df -Th /
    EOF
    user "root"
    live_stream true
    ignore_failure true
  end

end


# This will copy the whole file tree from COOKBOOK_NAME/files/default/tests
# to the directory where RSpec is expecting them.
testdir = value_for_platform(
  'windows' => { 'default' => ::File.join(Chef::Config[:file_cache_path], 'system-probe-tests') },
  'default' => '/system-probe-tests'
)

if !platform?('windows')
    # retro compatibility
    execute "/tmp/system-probe-tests symlink" do
      command "ln -s /system-probe-tests /tmp/system-probe-tests"
      live_stream true
      action :run
      ignore_failure false
    end
end

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
