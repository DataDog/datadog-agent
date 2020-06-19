#
# Cookbook Name:: dd-agent-install-script
# Recipe:: default
#
# Copyright (C) 2013 Datadog
#
# All rights reserved - Do Not Redistribute
#

wrk_dir = node['dd-agent-install-script']['install_script_dir']

directory wrk_dir do
  recursive true
end

remote_file "#{wrk_dir}/install-script" do
  source node['dd-agent-install-script']['install_script_url']
end

# apt-get update fails a LOT on our droplets, so ignore these failures
# TODO: assess whether we should do the same thing in the install script itself
execute 'ignore "apt-get update" failure' do
  cwd wrk_dir
  command "sed -i 's/apt-get update$/apt-get update || true/' install-script"
end

kitchen_environment_variables = {
  'DD_API_KEY' => node['dd-agent-install-script']['api_key'],
  'REPO_URL' => node['dd-agent-install-script']['repo_url'],
  'DD_URL' => node['dd-agent-install-script']['dd_url'],
  'DD_SITE' => node['dd-agent-install-script']['dd_site'],
  'DD_AGENT_FLAVOR' => node['dd-agent-install-script']['agent_flavor'],

  'TESTING_APT_URL' => node['dd-agent-install-script']['repo_domain_apt'],
  'TESTING_YUM_URL' => node['dd-agent-install-script']['repo_domain_yum'],
  'TESTING_APT_REPO_VERSION' => "#{node['dd-agent-install-script']['repo_branch_apt']} #{node['dd-agent-install-script']['repo_component_apt']}",
  'TESTING_YUM_VERSION_PATH' => node['dd-agent-install-script']['repo_branch_yum'],
}.compact

# Transform hash into bash syntax for exporting environment variables
kitchen_env_export = kitchen_environment_variables.map{ |pair| "export '#{pair.join('=')}'" }.join("\n")

execute 'update Agent install script repository' do
  cwd wrk_dir
  command <<-EOF
    sed -i 's~$sudo_cmd which service~sudo which service~' install-script
  EOF

  only_if { node['dd-agent-install-script']['install_candidate'] }
end

user 'installuser' do
  comment 'Not a root user to run install script'
  uid 9999
  home '/home/installuser'
  shell '/bin/bash'
end

sudo 'installuser' do
  nopasswd true
  users 'installuser'
end

directory wrk_dir do
  owner 'installuser'
  mode '0755'
end

execute 'run agent install script' do
  user 'installuser'
  cwd wrk_dir
  command <<-EOF
    #{kitchen_env_export}
    bash install-script
    sleep 10
  EOF
  live_stream true
end
