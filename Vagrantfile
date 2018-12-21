Vagrant.configure("2") do |config|

  branch = "master" # master or PR_NAME

  #Ubuntu 14
  config.vm.define "trusty" do |trusty|
    trusty.vm.box = "ubuntu/trusty64"
    trusty.vm.hostname = 'trusty'
    trusty.vm.box_url = "ubuntu/trusty64"

    trusty.vm.network :private_network, ip: "192.168.56.101"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    trusty.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "trusty"]
    end
  end

  #Ubuntu 16.04
  config.vm.define "xenial" do |xenial|
    xenial.vm.box = "ubuntu/xenial64"
    xenial.vm.hostname = 'xenial'
    xenial.vm.box_url = "ubuntu/xenial64"

    xenial.vm.network :private_network, ip: "192.168.56.102"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    xenial.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "xenial"]
    end
  end

  #Ubuntu 18.04
  config.vm.define "bionic" do |bionic|
    bionic.vm.box = "ubuntu/bionic64"
    bionic.vm.hostname = 'bionic'
    bionic.vm.box_url = "ubuntu/bionic64"

    bionic.vm.network :private_network, ip: "192.168.56.103"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    bionic.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "bionic"]
    end
  end

  #Debian 7
  config.vm.define "wheezy" do |wheezy|
    wheezy.vm.box = "debian/wheezy64"
    wheezy.vm.hostname = 'wheezy'
    wheezy.vm.box_url = "debian/wheezy64"

    wheezy.vm.network :private_network, ip: "192.168.56.104"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    wheezy.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "wheezy"]
    end
  end

  #Debian 8
  config.vm.define "jessie" do |jessie|
    jessie.vm.box = "debian/jessie64"
    jessie.vm.hostname = 'jessie'
    jessie.vm.box_url = "debian/jessie64"

    jessie.vm.network :private_network, ip: "192.168.56.105"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    jessie.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "jessie"]
    end
  end

  #Debian 9
  config.vm.define "stretch" do |stretch|
    stretch.vm.box = "debian/stretch64"
    stretch.vm.hostname = 'stretch'
    stretch.vm.box_url = "debian/stretch64"

    stretch.vm.network :private_network, ip: "192.168.56.106"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "DEBIAN_REPO" => "https://stackstate-agent-2-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    stretch.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "stretch"]
    end
  end

  config.vm.define "centos7" do |centos7|
    centos7.vm.box = "centos/7"
    centos7.vm.hostname = 'centos7'
    centos7.vm.box_url = "centos/7"

    centos7.vm.network :private_network, ip: "192.168.56.110"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "YUM_REPO" => "https://stackstate-agent-2-rpm-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    centos7.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "centos7"]
    end
  end
  
  config.vm.define "rhel7" do |rhel7|
    rhel7.vm.box = "generic/rhel7"
    rhel7.vm.hostname = 'rhel7'
    rhel7.vm.box_url = "generic/rhel7"

    rhel7.vm.network :private_network, ip: "192.168.56.120"

    config.vm.synced_folder ".", "/opt/stackstate-agent-dev"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "YUM_REPO" => "https://stackstate-agent-2-rpm-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    rhel7.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "rhel7"]
    end
  end

  config.vm.define "fedora28" do |fedora28|
    fedora28.vm.box = "generic/fedora28"
    fedora28.vm.hostname = 'fedora28'
    fedora28.vm.box_url = "generic/fedora28"

    fedora28.vm.network :private_network, ip: "192.168.56.130"

    config.vm.provision "shell",
      env: {
        "STS_API_KEY" => "API_KEY",
        "STS_URL" => "http://192.168.56.1:7077/stsAgent",
        "YUM_REPO" => "https://stackstate-agent-2-rpm-test.s3.amazonaws.com",
        "CODE_NAME" => branch
      },
      path: "./cmd/agent/install_script.sh",
      privileged: false

    fedora28.vm.provider :virtualbox do |v|
      v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      v.customize ["modifyvm", :id, "--memory", 1028]
      v.customize ["modifyvm", :id, "--name", "fedora28"]
    end
  end

end
