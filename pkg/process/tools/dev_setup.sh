#!/usr/bin/env bash

cwd="$(dirname "$0")"
cd $cwd

set -e

if [[ -f Vagrantfile ]]; then
  echo "detected vagrant file; will clean up"
  vagrant destroy -f
  rm Vagrantfile
fi

cat <<EOD > Vagrantfile
Vagrant.configure("2") do |config|
  config.vm.box = "hashicorp-vagrant/ubuntu-16.04"
  config.vm.synced_folder "$GOPATH/src/github.com/DataDog", "/home/vagrant/go/src/github.com/DataDog"
end
EOD

vagrant up

# install invoke
cat <<EOD | vagrant ssh
sudo pip install invoke
EOD

# fix permissions
cat <<EOD | vagrant ssh
sudo chown vagrant /home/vagrant/go
sudo chown vagrant /home/vagrant/go/src/
sudo chown vagrant /home/vagrant/go/src/github.com
EOD

# golang setup
cat <<EOD | vagrant ssh
mkdir -p /opt
wget -q https://dl.google.com/go/go1.10.1.linux-amd64.tar.gz
sudo tar -xzf go1.10.1.linux-amd64.tar.gz -C /opt
echo 'export GOPATH=/home/vagrant/go' > ~/.bashrc
echo 'export PATH=/opt/go/bin:/home/vagrant/go/bin:\$PATH' >> ~/.bashrc

sudo wget https://github.com/golang/dep/releases/download/v0.5.0/dep-linux-386 -O /usr/local/bin/dep -q
sudo chmod +x /usr/local/bin/dep
EOD

# protoc setup
cat <<EOD | vagrant ssh
wget https://github.com/protocolbuffers/protobuf/releases/download/v3.3.0/protoc-3.3.0-linux-x86_64.zip -q
sudo unzip protoc-3.3.0-linux-x86_64.zip -d /opt/protobuf
echo 'export PATH=\$PATH:/opt/protobuf/bin' >> ~/.bashrc
go get github.com/gogo/protobuf/protoc-gen-gogofaster
EOD

# docker setup
cat <<EOD | vagrant ssh
sudo apt-get update
sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common ruby mercurial
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo sh -c 'echo "deb https://download.docker.com/linux/ubuntu xenial stable" >> /etc/apt/sources.list'
sudo apt-get update
sudo apt-get install -y docker-ce
sudo groupadd docker
sudo usermod -aG docker vagrant
sudo service docker start
EOD

# clang setup
cat <<EOD | vagrant ssh
sudo apt-get install -y clang-format
EOD

# necessary to get group membership to be respected
vagrant reload

echo "your development environment is ready; use \`VAGRANT_CWD=$cwd vagrant ssh\` to ssh in"
