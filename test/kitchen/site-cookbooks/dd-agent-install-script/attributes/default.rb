default['dd-agent-install-script']['api_key'] = nil
default['dd-agent-install-script']['install_script_dir'] = '/tmp/install-script/'
default['dd-agent-install-script']['install_script_url'] = 'https://raw.githubusercontent.com/DataDog/datadog-agent/master/cmd/agent/install_script.sh'
default['dd-agent-install-script']['install_candidate'] = true

default['dd-agent-install-script']['repo_url'] = 'datad0g.com'
default['dd-agent-install-script']['dd_url'] = 'datad0g.com'
default['dd-agent-install-script']['dd_site'] = 'datadoghq.eu'
default['dd-agent-install-script']['agent_flavor'] = 'datadog-agent'

default['dd-agent-install-script']['repo_domain_yum'] = 'yumtesting.datad0g.com'
default['dd-agent-install-script']['repo_branch_yum'] = 'testing'

default['dd-agent-install-script']['repo_domain_apt'] = 'apttesting.datad0g.com'
default['dd-agent-install-script']['repo_branch_apt'] = 'testing'
default['dd-agent-install-script']['repo_component_apt'] = 'main'
