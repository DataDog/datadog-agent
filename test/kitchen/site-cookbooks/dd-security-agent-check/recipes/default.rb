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

  cookbook_file "#{wrk_dir}/nikos.tar.gz" do
    source "nikos.tar.gz"
    mode '755'
  end

  execute "Extract nikos.tar.gz" do
    command "mkdir -p /opt/datadog-agent/embedded/nikos/embedded/ && tar -xzvf #{wrk_dir}/nikos.tar.gz -C /opt/datadog-agent/embedded/nikos/embedded/"
    action :run
  end

  # `/swapfile` doesn't work on Oracle Linux, so we use `/mnt/swapfile`
  swap_file '/mnt/swapfile' do
    size 2048
  end

  kernel_module 'loop' do
    action :load
  end

  kernel_module 'veth' do
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

    if ['oracle'].include?(node[:platform])
      docker_installation_package 'default' do
        action :create
        setup_docker_repo false
        package_name 'docker-engine'
        package_options %q|-y|
      end

      service 'docker' do
        action [ :enable, :start ]
      end
    elsif ['ubuntu'].include?(node[:platform])
      docker_installation_package 'default' do
        action :create
        setup_docker_repo false
        package_name 'docker.io'
      end
    else
      docker_service 'default' do
        action [:create, :start]
      end
    end

    file "#{wrk_dir}/Dockerfile" do
      content <<-EOF
      FROM centos:7
      ENV DOCKER_DD_AGENT=yes
      ADD nikos.tar.gz /opt/datadog-agent/embedded/nikos/embedded/
      RUN yum -y install xfsprogs e2fsprogs iproute
      CMD sleep 7200
      EOF
      action :create
    end

    docker_image 'testsuite-img' do
      tag 'latest'
      source wrk_dir
      action :build
    end

    docker_container 'docker-testsuite' do
      repo 'testsuite-img'
      tag 'latest'
      cap_add ['SYS_ADMIN', 'SYS_RESOURCE', 'SYS_PTRACE', 'NET_ADMIN', 'IPC_LOCK', 'ALL']
      volumes [
        # security-agent misc
        '/tmp/security-agent:/tmp/security-agent',
        # HOST_* paths
        '/proc:/host/proc',
        '/etc:/host/etc',
        '/sys:/host/sys',
        # os-release
        '/etc/os-release:/host/etc/os-release',
        '/usr/lib/os-release:/host/usr/lib/os-release',
        # passwd and groups
        '/etc/passwd:/etc/passwd',
        '/etc/group:/etc/group',
      ]
      env [
        'HOST_PROC=/host/proc',
        'HOST_ETC=/host/etc',
        'HOST_SYS=/host/sys',
      ]
      privileged true
    end

    docker_exec 'debug_fs' do
      container 'docker-testsuite'
      command ['mount', '-t', 'debugfs', 'none', '/sys/kernel/debug']
    end

    for i in 0..7 do
      docker_exec 'create_loop' do
        container 'docker-testsuite'
        command ['bash', '-c', "mknod /dev/loop#{i} b 7 #{i} || true"]
      end
    end
  end

  if platform_family?('centos', 'fedora', 'rhel')
    selinux_state "SELinux Permissive" do
      action :permissive
    end
  end

  if File.exists?('/sys/kernel/security/lockdown')
    file '/sys/kernel/security/lockdown' do
      action :create_if_missing
      content "integrity"
    end
  end
end
