return unless platform_family?('rhel')

include_recipe 'yum-centos::default'

node['yum-centos']['vault_repos'].each do |release, _config|
  node['yum-centos']['repos'].each do |id|
    next unless node['yum'][id]['managed']
    next unless node['yum-centos']['vault_repos'][release]['managed']
    dir =
      case id
      when 'base'
        value_for_platform(%w(centos redhat) =>
          {
            '>= 8.0' => 'BaseOS',
            '< 8.0' => 'os',
          })
      when 'appstream'
        'AppStream'
      when 'powertools'
        'PowerTools'
      when 'updates', 'extras', 'centosplus', 'fasttrack'
        id
      else
        next
      end

    yum_repository "centos-vault-#{release}-#{id}" do
      description "CentOS-#{release} Vault - #{id.capitalize}"
      case node['platform_version'].to_i
      when 7
        if arm?
            baseurl "http://vault.centos.org/altarch/#{release}/#{dir}/$basearch/"
            gpgkey ['file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever-$basearch', 'file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever']
        else
            baseurl "http://vault.centos.org/#{release}/#{dir}/$basearch/"
            gpgkey 'file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-$releasever'
        end
      when 8
        baseurl "http://vault.centos.org/#{release}/#{dir}/$basearch/os/"
        gpgkey 'file:///etc/pki/rpm-gpg/RPM-GPG-KEY-centosofficial'
      end
      node['yum-centos']['vault_repos'][release].each do |config, value|
        case config
        when 'managed' # rubocop: disable Lint/EmptyWhen
        when 'baseurl'
          send(config.to_sym, lazy { value })
        else
          send(config.to_sym, value) unless value.nil?
        end
      end
    end
  end
end
