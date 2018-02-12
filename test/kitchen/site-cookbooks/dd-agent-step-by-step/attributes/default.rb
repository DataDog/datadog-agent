default['dd-agent-step-by-step']['api_key'] = 'apikey_2'
default['dd-agent-step-by-step']['install_candidate'] = false
default['dd-agent-step-by-step']['package_name'] = 'datadog-agent'

if node['dd-agent-step-by-step']['install_candidate']
  default['dd-agent-step-by-step']['repo_domain_apt'] = 'apttesting.datad0g.com'
  default['dd-agent-step-by-step']['repo_domain_yum'] = 'yumtesting.datad0g.com'
  default['dd-agent-step-by-step']['repo_branch_apt'] = "testing"
  default['dd-agent-step-by-step']['repo_branch_yum'] = "testing"
else
  default['dd-agent-step-by-step']['repo_domain_apt'] = 'apt.datadoghq.com'
  default['dd-agent-step-by-step']['repo_domain_yum'] = 'yum.datadoghq.com'
  default['dd-agent-step-by-step']['repo_branch_apt'] = "stable"
  default['dd-agent-step-by-step']['repo_branch_yum'] = "rpm"
end
