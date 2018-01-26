default['dd-agent-install-script']['api_key'] = nil
default['dd-agent-install-script']['working_dir'] = '/tmp/install-script/'
default['dd-agent-install-script']['install_script_url'] = 'https://raw.githubusercontent.com/DataDog/datadog-agent/master/cmd/agent/install_script.sh'
default['dd-agent-install-script']['install_candidate'] = true

default['dd-agent-install-script']['candidate_repo_domain_apt'] = 'apttesting.datad0g.com'
default['dd-agent-install-script']['candidate_repo_domain_yum'] = 'yumtesting.datad0g.com'
default['dd-agent-install-script']['candidate_repo_branch'] = 'testing'
