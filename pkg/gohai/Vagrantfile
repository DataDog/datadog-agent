# -*- mode: ruby -*-
# vi: set ft=ruby :

# Vagrantfile API/syntax version. Don't touch unless you know what you're doing!
VAGRANTFILE_API_VERSION = "2"

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|
  config.vm.box = "CentOS 6.4 x86_64 Minimal (VirtualBox Guest Additions 4.2.16, Chef 11.6.0, Puppet 3.2.3)"
  config.vm.box_url = "http://developer.nrel.gov/downloads/vagrant-boxes/CentOS-6.4-x86_64-v20130731.box"

  config.vm.provision :shell, inline: %[echo "export PATH=/usr/local/go/bin:/home/vagrant/go/bin:\$PATH\nexport GOPATH=/home/vagrant/go" > /etc/profile.d/go.sh]
  config.vm.provision :shell, inline: %[chmod +x /etc/profile.d/go.sh]
  config.vm.provision :shell, inline: %[mkdir /home/vagrant/go]
  config.vm.provision :shell, inline: %[chown vagrant:vagrant /home/vagrant/go]
end
