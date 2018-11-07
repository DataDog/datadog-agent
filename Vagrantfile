Vagrant.configure("2") do |config|

  config.vm.define "xenial" do |xenial|
    xenial.vm.box = "ubuntu/xenial64"
    xenial.vm.hostname = 'xenial'
    xenial.vm.box_url = "ubuntu/xenial64"

    xenial.vm.network :private_network, ip: "192.168.56.101"

    # config.vm.synced_folder ".", "/opt/stackstate-agent"

    xenial.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 512]
      v.customize ["modifyvm", :id, "--name", "xenial"]
    end
  end

  config.vm.define "bionic" do |bionic|
    bionic.vm.box = "ubuntu/bionic64"
    bionic.vm.hostname = 'bionic'
    bionic.vm.box_url = "ubuntu/bionic64"

    bionic.vm.network :private_network, ip: "192.168.56.102"

    # config.vm.synced_folder ".", "/opt/stackstate-agent"

    bionic.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 512]
      v.customize ["modifyvm", :id, "--name", "bionic"]
    end
  end

  config.vm.define "jessie" do |jessie|
    jessie.vm.box = "debian/jessie64"
    jessie.vm.hostname = 'jessie'
    jessie.vm.box_url = "debian/jessie64"

    jessie.vm.network :private_network, ip: "192.168.56.103"

    # config.vm.synced_folder ".", "/opt/stackstate-agent"

    jessie.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 512]
      v.customize ["modifyvm", :id, "--name", "jessie"]
    end
  end

  config.vm.define "stretch" do |stretch|
    stretch.vm.box = "debian/stretch64"
    stretch.vm.hostname = 'stretch'
    stretch.vm.box_url = "debian/stretch64"

    stretch.vm.network :private_network, ip: "192.168.56.104"

    # config.vm.synced_folder ".", "/opt/stackstate-agent"

    stretch.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 512]
      v.customize ["modifyvm", :id, "--name", "stretch"]
    end
  end

end
