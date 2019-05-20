#!/usr/bin/env bash

cwd="$(dirname "$0")"
cd $cwd

set -ex

VAGRANT_CWD=$(pwd)

if [[ -f Vagrantfile ]]; then
    echo "detected vagrant file; will clean up"
    vagrant destroy -f
    rm Vagrantfile
fi

cat <<EOD > Vagrantfile
Vagrant.configure("2") do |config|
  config.vm.box = "hashicorp-vagrant/ubuntu-16.04"
  config.vbguest.auto_update = false
  config.vm.synced_folder "$GOPATH/src/github.com/DataDog", "/home/vagrant/go/src/github.com/DataDog"
end
EOD

vagrant up


echo "installing tools (invoke, clang format, jq, vim)"
cat <<EOD | vagrant ssh
sudo apt-get update
sudo apt-get install -y python-pip unzip curl jq vim clang-format --fix-missing
sudo pip install invoke pyyaml
EOD

echo "installing clang and LLVM v8"
cat <<EOD | vagrant ssh
wget -O - https://apt.llvm.org/llvm-snapshot.gpg.key | sudo apt-key add -
echo "deb http://apt.llvm.org/trusty/ llvm-toolchain-trusty-8 main" | sudo tee -a /etc/apt/sources.list
echo "deb-src http://apt.llvm.org/trusty/ llvm-toolchain-trusty-8 main" | sudo tee -a /etc/apt/sources.list
sudo apt-get update
sudo apt-get install -y clang-8 llvm-8
sudo ln -sf /usr/bin/clang-8 /usr/bin/clang
sudo ln -sf /usr/bin/llc-8 /usr/bin/llc
EOD

echo "installing linux-headers"
cat <<EOD | vagrant ssh
sudo apt-get install -y linux-headers-\$(uname -r)
EOD

echo "fixing permissions"
cat <<EOD | vagrant ssh
sudo chown vagrant /home/vagrant/go
sudo chown vagrant /home/vagrant/go/src/
sudo chown vagrant /home/vagrant/go/src/github.com
EOD

echo "golang setup"
cat <<EOD | vagrant ssh
mkdir -p /opt
wget -q https://dl.google.com/go/go1.11.5.linux-amd64.tar.gz
sudo tar -xzf go1.11.5.linux-amd64.tar.gz -C /opt
echo 'export GOPATH=/home/vagrant/go' > ~/.bashrc
echo 'export PATH=/opt/go/bin:/home/vagrant/go/bin:\$PATH' >> ~/.bashrc
sudo wget https://github.com/golang/dep/releases/download/v0.5.0/dep-linux-386 -O /usr/local/bin/dep -q
sudo chmod +x /usr/local/bin/dep
EOD

echo "protobuf setup"
cat <<EOD | vagrant ssh
wget https://github.com/protocolbuffers/protobuf/releases/download/v3.3.0/protoc-3.3.0-linux-x86_64.zip -q
sudo unzip protoc-3.3.0-linux-x86_64.zip -d /opt/protobuf
echo 'export PATH=\$PATH:/opt/protobuf/bin' >> ~/.bashrc
go get github.com/gogo/protobuf/protoc-gen-gogofaster
EOD

echo "docker setup"
cat <<EOD | vagrant ssh
sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common ruby mercurial
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
echo "deb https://download.docker.com/linux/ubuntu xenial stable" | sudo tee -a /etc/apt/sources.list
sudo apt-get update
sudo apt-get install -y docker-ce
sudo groupadd docker
sudo usermod -aG docker vagrant
sudo service docker start
EOD

echo "Creating directories and config files"
cat <<EOD | vagrant ssh
sudo mkdir -p /opt/datadog-agent/run/
sudo mkdir -p /etc/datadog-agent/
echo "
log_level: "debug"
log_to_console: true

network_tracer_config:
  enabled: true
  log_file: "/home/vagrant/network-tracer.log"
  bpf_debug: true
" | sudo tee /etc/datadog-agent/network-tracer.yaml
EOD

echo "Adding some aliases and login help"
cat <<EOD | vagrant ssh
echo 'alias goforit="cd /home/vagrant/go/src/github.com/DataDog/datadog-agent"' >> ~/.bashrc
echo 'alias curl_agent="curl -s --unix-socket /opt/datadog-agent/run/nettracer.sock"'
echo 'echo "----------------------------------------------
Hi and welcome in the ebpf dev env, quick help:
- \`goforit\` to go to the datadog-agent root dir
- \`invoke -e deps\` to retrieve the required dependencies
- \`invoke -e network-tracer.build\` to build a network-tracer (will be in ./bin/network-tracer/network-tracer)
- \`invoke -e network-tracer.nettop\` to run nettop (a simple eBPF connection logger)
- \`curl_agent\` http://unix/debug/stats to query endpoints on the network-tracer
----------------------------------------------"' >> ~/.profile
EOD

# necessary to get group membership to be respected
vagrant reload

 echo "your development environment is ready; use \`VAGRANT_CWD=$cwd vagrant ssh\` to ssh in"
