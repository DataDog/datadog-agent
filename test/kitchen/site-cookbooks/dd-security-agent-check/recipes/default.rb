#
# Cookbook Name:: dd-security-agent-check
# Recipe:: default
#
# Copyright (C) 2020 Datadog
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

  cookbook_file "#{wrk_dir}/testsuite32" do
    source "testsuite32"
    mode '755'
  end

  docker_service 'default' do
    action [:create, :start]
  end

  docker_image 'debian' do
    tag 'bullseye'
    action :pull
  end

  docker_container 'docker-testsuite' do
    repo 'debian'
    tag 'bullseye'
    cap_add ['SYS_ADMIN', 'SYS_RESOURCE', 'SYS_PTRACE', 'NET_ADMIN', 'IPC_LOCK', 'ALL']
    command "sleep 3600"
    volumes '/tmp/security-agent:/tmp/security-agent'
    privileged true
  end

  docker_exec 'debug_fs' do
    container 'docker-testsuite'
    command ['mount', '-t', 'debugfs', 'none', '/sys/kernel/debug']
  end

  package 'Install i386 libc' do
    case node[:platform]
    when 'redhat', 'centos', 'suse', 'fedora'
      package_name 'glibc.i686'
    when 'ubuntu', 'debian'
      package_name 'libc6-i386'
    end
  end

  kernel_module 'loop' do
    action :load
  end
end