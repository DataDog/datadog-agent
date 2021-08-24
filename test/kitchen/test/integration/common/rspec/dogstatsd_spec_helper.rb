require 'spec_helper'

shared_examples_for 'Dogstatsd install' do
  it_behaves_like 'an installed Dogstatsd'
  it_behaves_like 'an installed Datadog Signing Keys'
end

shared_examples_for 'Dogstatsd behavior' do
  it_behaves_like 'a running Dogstatsd'
  it_behaves_like 'a Dogstatsd that stops'
  it_behaves_like 'a Dogstatsd that restarts'
end

shared_examples_for 'Dogstatsd uninstall' do
  it_behaves_like 'a Dogstatsd that is removed'
end

shared_examples_for "an installed Dogstatsd" do
  wait_until_service_started get_service_name("datadog-dogstatsd")

  it 'has an example config file' do
    if os != :windows
      expect(File).to exist('/etc/datadog-dogstatsd/dogstatsd.yaml.example')
    end
  end
  
  it 'has a datadog-agent binary in usr/bin' do
    if os != :windows
      expect(File).to exist('/usr/bin/datadog-dogstatsd')
    end
  end
end

shared_examples_for "a running Dogstatsd" do
  it 'is running' do
    expect(dogstatsd_processes_running?).to be_truthy
  end

  it 'has a config file' do
    if os == :windows
      conf_path = "#{ENV['ProgramData']}\\Datadog\\dogstatsd.yaml"
    else
      conf_path = '/etc/datadog-dogstatsd/dogstatsd.yaml'
    end
    expect(File).to exist(conf_path)
  end
end

shared_examples_for 'a Dogstatsd that stops' do
  it 'stops' do
    output = stop "datadog-dogstatsd"
    if os != :windows
      expect(output).to be_truthy
    end
    expect(is_flavor_running? "datadog-dogstatsd").to be_falsey
  end
  
  it 'is not running any dogstatsd processes' do
    expect(dogstatsd_processes_running?).to be_falsey
  end

  it 'starts after being stopped' do
    output = start "datadog-dogstatsd"
    if os != :windows
      expect(output).to be_truthy
    end
    expect(is_flavor_running? "datadog-dogstatsd").to be_truthy
    end
  end
  
  shared_examples_for 'a Dogstatsd that restarts' do
    it 'restarts when dogstatsd is running' do
      if !is_flavor_running? "datadog-dogstatsd"
        start "datadog-dogstatsd"
      end
      output = restart "datadog-dogstatsd"
      if os != :windows
        expect(output).to be_truthy
      end
      expect(is_flavor_running? "datadog-dogstatsd").to be_truthy
    end
  
    it 'restarts when dogstatsd is not running' do
      if is_flavor_running? "datadog-dogstatsd"
        stop "datadog-dogstatsd"
      end
      output = restart "datadog-dogstatsd"
      if os != :windows
        expect(output).to be_truthy
      end
      expect(is_flavor_running? "datadog-dogstatsd").to be_truthy
    end
end

shared_examples_for 'a Dogstatsd that is removed' do
  it 'should remove Dogstatsd' do
    if os == :windows
      # Placeholder, not used yet
      # uninstallcmd = "start /wait msiexec /q /x 'C:\\Users\\azure\\AppData\\Local\\Temp\\kitchen\\cache\\ddagent-cli.msi'"
      uninstallcmd='for /f "usebackq" %n IN (`wmic product where "name like \'datadog%\'" get IdentifyingNumber ^| find "{"`) do start /wait msiexec /log c:\\uninst.log /q /x %n'
      expect(system(uninstallcmd)).to be_truthy
    else
      if system('which apt-get &> /dev/null')
        expect(system("sudo apt-get -q -y remove datadog-dogstatsd > /dev/null")).to be_truthy
      elsif system('which yum &> /dev/null')
        expect(system("sudo yum -y remove datadog-dogstatsd > /dev/null")).to be_truthy
      elsif system('which zypper &> /dev/null')
        expect(system("sudo zypper --non-interactive remove datadog-dogstatsd > /dev/null")).to be_truthy
      else
        raise 'Unknown package manager'
      end
    end
  end

  it 'should not be running after removal' do
    sleep 5
    expect(dogstatsd_processes_running?).to be_falsey
  end

  it 'should remove the installation directory' do
    if os == :windows
      # Placeholder, not used yet
      expect(File).not_to exist("C:\\Program Files\\Datadog\\Datadog Dogstatsd\\")
    else
      expect(File).not_to exist("/opt/datadog-dogstatsd/")
    end
  end

  if os != :windows
    it 'should remove the dogstatsd link from bin' do
      expect(File).not_to exist('/usr/bin/datadog-dogstatsd')
    end
  end
end

