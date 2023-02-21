name "librdkafka"
default_version "2.0.2"

source :url => "https://github.com/confluentinc/librdkafka/archive/refs/tags/v#{version}.tar.gz",
       :sha256 => "f321bcb1e015a34114c83cf1aa7b99ee260236aab096b85c003170c90a47ca9d",
       :extract => :seven_zip

relative_path "librdkafka-#{version}"

build do

  license "BSD-style"
  license_file "https://raw.githubusercontent.com/confluentinc/librdkafka/master/LICENSE"

  # https://github.com/confluentinc/confluent-kafka-python/blob/master/INSTALL.md#install-from-source

  if windows?
    command "vcpkg install librdkafka"
  elsif redhat?
    command "yum install -y cyrus-sasl-gssapi krb5-workstation"

    command "rpm --import https://packages.confluent.io/rpm/7.3/archive.key"

    command "echo '[Confluent-Clients]' >> /etc/yum.repos.d/confluent.repo"
    command "echo 'name=Confluent Clients repository' >> /etc/yum.repos.d/confluent.repo"
    command "echo 'baseurl=https://packages.confluent.io/clients/rpm/centos/$releasever/$basearch' >> /etc/yum.repos.d/confluent.repo"
    command "echo 'gpgcheck=1' >> /etc/yum.repos.d/confluent.repo"
    command "echo 'gpgkey=https://packages.confluent.io/clients/rpm/archive.key' >> /etc/yum.repos.d/confluent.repo"
    command "echo 'enabled=1' >> /etc/yum.repos.d/confluent.repo"

    command "yum install -y librdkafka-devel"
  elsif debian?
    command "apt install -y libsasl2-modules-gssapi-mit krb5-user"
    command "wget -qO - https://packages.confluent.io/deb/7.3/archive.key | apt-key add -"

    command "add-apt-repository \"deb https://packages.confluent.io/clients/deb $(lsb_release -cs) main\""
    command "apt update"
    command "apt install -y librdkafka-dev"
  elsif osx?
    command "brew install librdkafka"
  end
end
