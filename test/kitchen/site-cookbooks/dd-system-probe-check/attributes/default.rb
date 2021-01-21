if platform?('centos')
  default['yum-centos']['vault_repos'][node[:platform_version]]['enabled'] = true
  default['yum-centos']['vault_repos'][node[:platform_version]]['managed'] = true
  default['yum-centos']['vault_repos'][node[:platform_version]]['make_cache'] = true
end

