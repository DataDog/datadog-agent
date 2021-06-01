#
# Cookbook Name:: dd-security-agent-check
# Recipe:: default
#
# Copyright (C) 2020-present Datadog
#

if node['platform_family'] != 'windows'
  wrk_dir = '/tmp/security-agent'

  directory wrk_dir do
    recursive true
  end

  cookbook_file "#{wrk_dir}/testsuite" do
    source "testsuite"
    mode '755'
  end

  # To uncomment when gitlab runner are able to build with GOARCH=386
  # cookbook_file "#{wrk_dir}/testsuite32" do
  #   source "testsuite32"
  #   mode '755'
  # end

  kernel_module 'loop' do
    action :load
  end

  if not ['redhat', 'suse', 'opensuseleap'].include?(node[:platform])
    if ['ubuntu', 'debian'].include?(node[:platform])
      apt_update

      package 'gnupg'

      package 'unattended-upgrades' do
        action :remove
      end
    end

    if ['ubuntu', 'debian', 'centos'].include?(node[:platform])
      package 'xfsprogs'
    end

    docker_service 'default' do
      action [:create, :start]
    end

    docker_image 'centos' do
      tag '7'
      action :pull
    end

    docker_container 'docker-testsuite' do
      repo 'centos'
      tag '7'
      cap_add ['SYS_ADMIN', 'SYS_RESOURCE', 'SYS_PTRACE', 'NET_ADMIN', 'IPC_LOCK', 'ALL']
      command "sleep 3600"
      volumes ['/tmp/security-agent:/tmp/security-agent', '/proc:/host/proc', '/etc/os-release:/host/etc/os-release']
      env ['HOST_PROC=/host/proc', 'DOCKER_DD_AGENT=yes']
      privileged true
      pid_mode 'host'
    end

    docker_exec 'debug_fs' do
      container 'docker-testsuite'
      command ['mount', '-t', 'debugfs', 'none', '/sys/kernel/debug']
    end

    docker_exec 'install_xfs' do
      container 'docker-testsuite'
      command ['yum', '-y', 'install', 'xfsprogs', 'e2fsprogs']
    end

    for i in 0..7 do
      docker_exec 'create_loop' do
        container 'docker-testsuite'
        command ['bash', '-c', "mknod /dev/loop#{i} b 7 #{i} || true"]
      end
    end
  end

  if not platform_family?('suse', 'rhel')
    package 'Install i386 libc' do
      case node[:platform]
      when 'redhat', 'centos', 'fedora'
        package_name 'glibc.i686'
      when 'ubuntu', 'debian'
        package_name 'libc6-i386'
      # when 'suse'
      #   package_name 'glibc-32bit'
      end
    end
  end

  if platform_family?('centos', 'fedora', 'rhel')
    selinux_state "SELinux Permissive" do
      action :permissive
    end
  end
end
