agent_version = {
    ## for dev
    :branch => "master",              # or use the PR_NAME
    :repo_suffix => "-test",

    ## for stable
    # :branch => "stable",
    # :repo_suffix => "",
}

machines = {
    :trusty => {
        :box => 'ubuntu/trusty64',    # Ubuntu 14
        :ip => '192.168.56.101',
        # :share_cwd => false,        # Defaults can be overridden
        # :install => true,
    },
    :xenial => {
        :box => 'ubuntu/xenial64',    # Ubuntu 16
        :ip => '192.168.56.102',
    },
    :bionic => {
        :box => 'ubuntu/bionic64',    # Ubuntu 18
        :ip => '192.168.56.103',
    },
    :wheezy => {
        :box => 'debian/wheezy64',    # Debian 7, Needs glibc 2.17
        :ip => '192.168.56.104',
    },
    :jessie => {
        :box => 'debian/jessie64',    # Debian 8
        :ip => '192.168.56.105',
    },
    :stretch => {
        :box => 'debian/stretch64',   # Debian 9
        :ip => '192.168.56.106',
    },
    :centos6 => {
        :box => 'centos/6',
        :ip => '192.168.56.110',
    },
    :centos7 => {
        :box => 'centos/7',
        :ip => '192.168.56.111',
    },
    :rhel7 => {
        :box => 'generic/rhel7',
        :ip => '192.168.56.112',
    },
    :fedora28 => {
        :box => 'generic/fedora28',
        :ip => '192.168.56.113',
    },
    :win16 => {
        :box => 'mwrock/Windows2016',
        :ip => '192.168.56.120',
        :share_cwd => true,
    },
    :win12 => {
        :box => 'mwrock/Windows2012R2',
        :ip => '192.168.56.121',
        :share_cwd => true,
    },
}

Vagrant.configure("2") do |config|
  machines.each do |hostname, properties|

    config.vm.define hostname do |box|
      box.vm.box = properties[:box]
      box.vm.hostname = hostname
      box.vm.box_url = properties[:box]
      # box.vm.box_version = ""

      box.vm.network :private_network, ip: properties[:ip]
      box.vm.network "forwarded_port", guest: 8126, host: 8126

      # Does not share . by default unless :share_cwd => true
      box.vm.synced_folder '.', '/vagrant', disabled: !properties[:share_cwd]

      # Configure winrm if booting windows machine
      # if "#{hostname}".start_with?("win")
      #   # box.vm.communicator = "winrm"
      #   box.winrm.username = "vagrant"
      #   box.winrm.password = "vagrant"
      #   # Allow to use basic auth for login
      #   box.vm.provision "shell", inline: "Set-Item -Path WSMan:\\localhost\\Service\\Auth\\Basic -Value $true"
      #   box.vm.provision "shell", inline: "choco feature enable -n=allowGlobalConfirmation"
      #   # Install powershell 6 with linux remoting
      #   box.vm.provision "shell", inline: "iex ((New-Object System.Net.WebClient).DownloadString('https://raw.githubusercontent.com/softasap/sa-win/master/GetPowershell6LinuxRemoting.ps1'))"
      # end

      # Installs the agent by default unless :install => false
      if !properties.key?(:install) or properties[:install]

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
          if properties[:share_cwd]
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
        end

      end

      box.vm.provider :virtualbox do |v|
        v.memory = 2048
        v.cpus = 4
        # v.memory = 16384
        # v.cpus = 4
      end
    end

  end
end
