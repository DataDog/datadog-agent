if platform?('centos')
  default['yum-centos']['vault_repos'][node[:platform_version]]['enabled'] = true
  default['yum-centos']['vault_repos'][node[:platform_version]]['managed'] = true
  default['yum-centos']['vault_repos'][node[:platform_version]]['make_cache'] = true

  if Chef::SystemProbeHelpers::arm?(node)
    if node['platform_version'].to_i < 8
      default['yum']['base']['gpgkey'] = ['file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever-$basearch', 'file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever']
      default['yum']['updates']['gpgkey'] = ['file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever-$basearch', 'file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever']
      default['yum']['extras']['gpgkey'] = ['file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever-$basearch', 'file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever']
    end
  end
end

