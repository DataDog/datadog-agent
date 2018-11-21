Vagrant.configure("2") do |config|

  #Ubuntu 16.04
  config.vm.define "xenial" do |xenial|
    xenial.vm.box = "ubuntu/xenial64"
    xenial.vm.hostname = 'xenial'
    xenial.vm.box_url = "ubuntu/xenial64"

    xenial.vm.network :private_network, ip: "192.168.56.101"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.101:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => "master" # or PR_NAME
      },
      path: "./cmd/agent/install_script.sh"

    xenial.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 512]
      v.customize ["modifyvm", :id, "--name", "xenial"]
    end
  end

  #Ubuntu 18.04
  config.vm.define "bionic" do |bionic|
    bionic.vm.box = "ubuntu/bionic64"
    bionic.vm.hostname = 'bionic'
    bionic.vm.box_url = "ubuntu/bionic64"

    bionic.vm.network :private_network, ip: "192.168.56.102"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.101:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => "master" # or PR_NAME
      },
      path: "./cmd/agent/install_script.sh"

    bionic.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 512]
      v.customize ["modifyvm", :id, "--name", "bionic"]
    end
  end

  #Debian 8
  config.vm.define "jessie" do |jessie|
    jessie.vm.box = "debian/jessie64"
    jessie.vm.hostname = 'jessie'
    jessie.vm.box_url = "debian/jessie64"

    jessie.vm.network :private_network, ip: "192.168.56.103"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.101:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => "master" # or PR_NAME
      },
      path: "./cmd/agent/install_script.sh"

    jessie.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 512]
      v.customize ["modifyvm", :id, "--name", "jessie"]
    end
  end

  #Debian 9
  config.vm.define "stretch" do |stretch|
    stretch.vm.box = "debian/stretch64"
    stretch.vm.hostname = 'stretch'
    stretch.vm.box_url = "debian/stretch64"

    stretch.vm.network :private_network, ip: "192.168.56.104"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.101:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => "master" # or PR_NAME
      },
      path: "./cmd/agent/install_script.sh"

    stretch.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 512]
      v.customize ["modifyvm", :id, "--name", "stretch"]
    end
  end

end
