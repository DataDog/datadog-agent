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

execute 'update Agent install script repository' do
  cwd wrk_dir
  command <<-EOF
    sed -i 's/apt\\.datadoghq\\.com/#{node['dd-agent-install-script']['repo_domain_apt']}/' install-script
    sed -i 's/yum\\.datadoghq\\.com/#{node['dd-agent-install-script']['repo_domain_yum']}/' install-script
    sed -i 's/apt.${repo_url}/#{node['dd-agent-install-script']['repo_domain_apt']}/' install-script
    sed -i 's/yum.${repo_url}/#{node['dd-agent-install-script']['repo_domain_yum']}/' install-script
    sed -i 's~stable/x86_64~#{node['dd-agent-install-script']['repo_branch_yum']}/x86_64~' install-script
    sed -i 's~rpm/x86_64~#{node['dd-agent-install-script']['repo_branch_yum']}/x86_64~' install-script
    sed -i 's~beta/x86_64~#{node['dd-agent-install-script']['repo_branch_yum']}/x86_64~' install-script
    sed -i 's~beta/$ARCHI~#{node['dd-agent-install-script']['repo_branch_yum']}/x86_64~' install-script
    sed -i 's~beta/$ARCHI~#{node['dd-agent-install-script']['repo_branch_yum']}/x86_64~' install-script
    sed -i 's~beta/$ARCHI~#{node['dd-agent-install-script']['repo_branch_yum']}/x86_64~' install-script
    sed -i 's~beta main~#{node['dd-agent-install-script']['repo_branch_apt']} main~' install-script
    sed -i 's~stable main~#{node['dd-agent-install-script']['repo_branch_apt']} main~' install-script
    sed -i 's~stable 6~#{node['dd-agent-install-script']['repo_branch_apt']} main~' install-script
    sed -i 's~stable/6~#{node['dd-agent-install-script']['repo_branch_yum']}~' install-script
    sed -i 's~${dd_agent_dist_channel} ${dd_agent_major_version}~#{node['dd-agent-install-script']['repo_branch_apt']} main~' install-script
    sed -i 's~${dd_agent_dist_channel}/${dd_agent_major_version}~#{node['dd-agent-install-script']['repo_branch_yum']}~' install-script
  EOF

  only_if { node['dd-agent-install-script']['install_candidate'] }
end

execute 'run agent install script' do
  cwd wrk_dir
  command <<-EOF
    sed -i '1aDD_API_KEY=#{node['dd-agent-install-script']['api_key']}' install-script
    sed -i '1aREPO_URL="datad0g.com"' install-script
    sed -i '1aDD_URL="datad0g.com"' install-script
    bash install-script
    sleep 10
  EOF
  live_stream true
end
