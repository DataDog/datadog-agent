agent_version = {
    ## for dev
    :branch => "master",      # or use the PR_NAME
    :repo_suffix => "-test",

    ## for stable
    # :branch => "stable",
    # :repo_suffix => "",
}

machines = {
    :trusty => {
        :box => 'ubuntu/trusty64',    #Ubuntu 14
        :ip => '192.168.56.101',
    },
    :xenial => {
        :box => 'ubuntu/xenial64',    #Ubuntu 16
        :ip => '192.168.56.102',
    },
    :bionic => {
        :box => 'ubuntu/bionic64',    #Ubuntu 18
        :ip => '192.168.56.103',
    },
    :wheezy => {
        :box => 'debian/wheezy64',    #Debian 7
        :ip => '192.168.56.104',
    },
    :jessie => {
        :box => 'ubuntu/jessie64',    #Debian 8
        :ip => '192.168.56.105',
    },
    :stretch => {
        :box => 'ubuntu/stretch64',   #Debian 9
        :ip => '192.168.56.106',
    },
    :centos7 => {
        :box => 'centos/7',
        :ip => '192.168.56.110',
    },
    :rhel7 => {
        :box => 'generic/rhel7',
        :ip => '192.168.56.111',
    },
    :fedora28 => {
        :box => 'generic/fedora28',
        :ip => '192.168.56.112',
    },
    :win16 => {
        :box => 'mwrock/Windows2016',
        :ip => '192.168.56.120',
    },
}

Vagrant.configure("2") do |config|
  machines.each do |hostname, properties|

    config.vm.define hostname do |box|
      box.vm.box = properties[:box]
      box.vm.hostname = hostname
      box.vm.box_url = properties[:box]

      box.vm.network :private_network, ip: properties[:ip]

      if !"#{hostname}".start_with?("win")
        box.vm.provision "shell",
                         env: {
                             :STS_API_KEY => "API_KEY",
                             :STS_URL => "http://192.168.56.1:7077/stsAgent",
                             :DEBIAN_REPO => "https://stackstate-agent-2#{agent_version[:repo_suffix]}.s3.amazonaws.com",
                             :YUM_REPO => "https://stackstate-agent-2-rpm#{agent_version[:repo_suffix]}.s3.amazonaws.com",
                             :CODE_NAME => agent_version[:branch]
                         },
                         path: "./cmd/agent/install_script.sh",
                         privileged: false
      else
        $script = <<-SCRIPT
        Import-Module c:\\vagrant\\cmd\\agent\\install_script.ps1
        install -stsApikey API_KEY -stsUrl http://192.168.56.1:7077/stsAgent -codeName #{agent_version[:branch]}
        SCRIPT

        box.vm.provision "shell",
                         env: {
                             :WIN_REPO => "https://stackstate-agent-2#{agent_version[:repo_suffix]}.s3.amazonaws.com/windows",
                         },
                         inline: $script,
                         privileged: true
      end

      box.vm.provider :virtualbox do |v|
        v.customize ["modifyvm", :id, "--name", hostname]
        v.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
        v.customize ["modifyvm", :id, "--memory", 2048]
      end
    end

  end
end
