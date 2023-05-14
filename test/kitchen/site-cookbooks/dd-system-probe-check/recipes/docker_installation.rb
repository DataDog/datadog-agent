case node[:platform]
when 'ubuntu', 'debian'
  package 'gnupg'

  package 'unattended-upgrades' do
    action :remove
  end

  package 'xfsprogs'
when 'centos'
  package 'xfsprogs'
when 'redhat'
  execute 'install docker-compose' do
    command <<-EOF
      dnf config-manager --add-repo=https://download.docker.com/linux/centos/docker-ce.repo
      dnf install -y docker-ce-19.03.13
    EOF
    user "root"
    live_stream true
  end
end

case node[:platform]
when 'amazon'
  docker_installation_package 'default' do
    action :create
    package_name 'docker'
    package_options %q|-y|
  end

  service 'docker' do
    action [ :enable, :start ]
  end
when 'ubuntu'
  docker_installation_package 'default' do
    action :create
    package_name 'docker.io'
  end
when 'redhat'
  docker_service 'default' do
    action [:start]
  end
else
  docker_service 'default' do
    action [:create, :start]
  end
end

execute 'install docker-compose' do
  command <<-EOF
    curl -SL https://github.com/docker/compose/releases/download/v2.12.2/docker-compose-$(uname -s | awk '{print tolower($0)}')-$(uname -m) -o /usr/bin/docker-compose
    chmod 0755 /usr/bin/docker-compose
  EOF
  user "root"
  live_stream true
end
