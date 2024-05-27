#
# Cookbook Name:: dd-security-agent-check
# Recipe:: functional-tests
#
# Copyright (C) 2020-present Datadog
#

cookbook_file "#{node['common']['work_dir']}/tests/testsuite" do
  source "tests/testsuite"
  mode '755'
end

if node[:platform] == "amazon" and node[:platform_version] == "2022"
  execute "increase /tmp size" do
    command "mount -o remount,size=10G /tmp/"
    live_stream true
    action :run
  end
end

file "#{node['common']['work_dir']}/cws_platform" do
  content node[:cws_platform].to_s || ""
  mode 644
end

remote_directory "#{node['common']['work_dir']}/ebpf_bytecode" do
  source 'ebpf_bytecode'
  sensitive true
  files_owner 'root'
  owner 'root'
end

directory "/opt/datadog-agent/embedded/bin" do
  recursive true
end

cookbook_file "/opt/datadog-agent/embedded/bin/clang-bpf" do
  source "clang-bpf"
  mode '0744'
  action :create
end

cookbook_file "#{node['common']['work_dir']}/clang-bpf" do
  source "clang-bpf"
  mode '0744'
  action :create
end

cookbook_file "/opt/datadog-agent/embedded/bin/llc-bpf" do
  source "llc-bpf"
  mode '0744'
  action :create
end

cookbook_file "#{node['common']['work_dir']}/llc-bpf" do
  source "llc-bpf"
  mode '0744'
  action :create
end

# Resources for getting test output into the Datadog CI product

directory "/go/bin" do
  recursive true
end

cookbook_file "/go/bin/gotestsum" do
  source "gotestsum"
  mode '0744'
  action :create
end

cookbook_file "/go/bin/test2json" do
  source "test2json"
  mode '0744'
  action :create
end

directory "/tmp/junit" do
  recursive true
end

cookbook_file "/tmp/junit/job_env.txt" do
  source "job_env.txt"
  mode '0444'
  action :create
  ignore_failure true
end

directory "/tmp/testjson" do
  recursive true
end

directory "/tmp/pkgjson" do
  recursive true
end

# End resources for getting test output into the Datadog CI product

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

# Some functional tests, TestProcessIdentifyInterpreter for example, require python
# Re: the container tests: Python comes with the Dockerfile, Perl needs to be installed manually
if (not ['redhat', 'oracle', 'rocky'].include?(node[:platform])) or node[:platform_version].start_with?("7")
  package 'python3'
end

if not ['redhat', 'suse', 'opensuseleap', 'rocky'].include?(node[:platform])
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
  elsif ['amazon'].include?(node[:platform])
    docker_installation_package 'default' do
      action :create
      setup_docker_repo false
      package_name 'docker'
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

  if ['docker', 'docker-fentry'].include?(node[:cws_platform])
    # Please see https://github.com/paulcacheux/cws-buildimages/blob/main/Dockerfile
    # for the definition of this base image.
    # If this successfully helps in reducing the amount of rate limits, this should be moved
    # to DataDog/datadog-agent-buildimages.
    file "#{node['common']['work_dir']}/Dockerfile" do
      content <<-EOF
      FROM ghcr.io/paulcacheux/cws-centos7@sha256:b16587f1cc7caebc1a18868b9fbd3823e79457065513e591352c4d929b14c426

      COPY clang-bpf /opt/datadog-agent/embedded/bin/
      COPY llc-bpf /opt/datadog-agent/embedded/bin/

      CMD sleep 7200
      EOF
      action :create
    end

    docker_image 'testsuite-img' do
      tag 'latest'
      source node['common']['work_dir']
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

# system-probe common
file "/tmp/color_idx" do
  content node[:color_idx].to_s
  mode 644
end
