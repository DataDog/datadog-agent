#
# Cookbook Name:: dd-agent-install-script
# Recipe:: default
#
# Copyright (C) 2013-present Datadog
#
# All rights reserved - Do Not Redistribute
#

wrk_dir = node['dd-agent-5']['working_dir']
is_windows = node['platform_family'] == 'windows'

directory wrk_dir do
  recursive true
end

remote_file "#{wrk_dir}/install-script" do
  source node['dd-agent-5']['install_script_url']
end

# apt-get update fails a LOT on our droplets, so ignore these failures
# TODO: assess whether we should do the same thing in the install script itself
execute 'ignore "apt-get update" failure' do
  cwd wrk_dir
  command "sed -i 's/apt-get update$/apt-get update || true/' install-script"
end


execute 'run agent install script' do
  cwd wrk_dir
  command <<-EOF
    sed -i '1aDD_API_KEY=#{node['dd-agent-5']['api_key']}' install-script
    bash install-script
    sleep 10
  EOF
  live_stream true
end

agent_config_file = ::File.join(node['dd-agent-5']['config_dir'], 'datadog.conf')
template agent_config_file do
  def template_vars
    dd_url = 'https://app.datadoghq.com'

    api_keys = [node['dd-agent-5']['api_key']]
    dd_urls = [dd_url]
    {
      :api_keys => api_keys,
      :dd_urls => dd_urls
    }
  end
  if is_windows
    owner 'Administrators'
    rights :full_control, 'Administrators'
    inherits false
  else
    owner 'dd-agent'
    group 'root'
    mode '640'
  end
  variables(
    if respond_to?(:lazy)
      lazy { template_vars }
    else
      template_vars
    end
  )
  sensitive true if Chef::Resource.instance_methods(false).include?(:sensitive)
end