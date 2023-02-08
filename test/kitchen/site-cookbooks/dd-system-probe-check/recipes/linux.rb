root_dir = "/tmp/ci/system-probe"
tests_dir = ::File.join(root_dir, "tests")

file "/tmp/color_idx" do
  content node[:color_idx].to_s
  mode 644
end

if platform?('centos')
  case node['platform_version'].to_i
  when 7
    if node['platform_version'] != '7.9.2009'
      include_recipe '::old_vault'
    end
  when 8
    include_recipe '::old_vault'
  end
end

case node[:platform]
  when 'ubuntu', 'debian'
    apt_update
end

execute "update yum repositories" do
  command "yum -y update"
  user "root"
  case node[:platform]
  when 'amazon'
    action :run
  else
    action :nothing
  end
end

kernel_version = `uname -r`.strip
package 'kernel headers' do
  case node[:platform]
  when 'redhat', 'centos', 'fedora', 'amazon'
    package_name "kernel-devel-#{kernel_version}"
  when 'ubuntu', 'debian'
    package_name "linux-headers-#{kernel_version}"
  end
end

package 'java' do
  case node[:platform]
  when 'redhat', 'centos', 'fedora', 'amazon'
    package_name 'java'
  when 'ubuntu', 'debian'
    package_name 'default-jre'
  end
end

package 'python3'

case node[:platform]
  when 'centos', 'redhat'
    package 'iptables'
end

package 'conntrack'

package 'netcat' do
  case node[:platform]
  when 'amazon'
    package_name 'nmap-ncat'
  when 'redhat', 'centos', 'fedora'
    package_name 'nc'
  when 'debian', 'ubuntu'
    package_name 'netcat-openbsd'
  else
    package_name 'netcat'
  end
end

package 'socat'

package 'wget'

package 'curl' do
  case node[:platform]
  when 'amazon'
    case node[:platform_version]
    when '2022'
      package_name 'curl-minimal'
    else
      package_name 'curl'
    end
  else
    package_name 'curl'
  end
end

package 'iptables'

# Enable IPv6 support
kernel_module 'ipv6' do
  action :load
end
execute 'sysctl net.ipv6.conf.all.disable_ipv6=0'

execute 'ensure conntrack is enabled' do
  command "iptables -I INPUT 1 -m conntrack --ctstate NEW,RELATED,ESTABLISHED -j ACCEPT"
  user "root"
  action :run
end

execute 'disable firewalld on redhat' do
  command "systemctl disable --now firewalld"
  user "root"
  ignore_failure true
  case node[:platform]
  when 'redhat'
    action :run
  else
    action :nothing
  end
end

directory "/opt/datadog-agent/embedded/bin" do
  recursive true
end

directory "/opt/datadog-agent/embedded/include" do
  recursive true
end

directory "#{tests_dir}/pkg/ebpf/bytecode/build/co-re/btf" do
  recursive true
end

cookbook_file "/opt/datadog-agent/embedded/bin/clang-bpf" do
  source "clang-bpf"
  mode '0744'
  action :create
end

cookbook_file "/opt/datadog-agent/embedded/bin/llc-bpf" do
  source "llc-bpf"
  mode '0744'
  action :create
end

cookbook_file "#{tests_dir}/pkg/ebpf/bytecode/build/co-re/btf/minimized-btfs.tar.xz" do
  source "minimized-btfs.tar.xz"
  action :create
end

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

cookbook_file "/tmp/junit/job_url.txt" do
  source "job_url.txt"
  mode '0444'
  action :create
  ignore_failure true
end

cookbook_file "/tmp/junit/tags.txt" do
  source "tags.txt"
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

# Install relevant packages for docker
include_recipe "::docker_installation"
docker_file_dir = "#{root_dir}/kitchen-dockers"
remote_directory docker_file_dir do
  source 'dockers'
  files_owner 'root'
  files_group 'root'
  files_mode '0750'
  action :create
  recursive true
end

# Load docker images
execute 'install docker-compose' do
  cwd docker_file_dir
  command <<-EOF
    for docker_file in $(ls); do
      echo docker load -i $docker_file
      docker load -i $docker_file
      rm -rf $docker_file
    done
  EOF
  user "root"
  live_stream true
end
