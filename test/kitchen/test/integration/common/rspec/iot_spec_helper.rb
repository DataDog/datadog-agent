require 'spec_helper'

# We retrieve the value defined in kitchen.yml because there is no simple way
# to set env variables on the target machine or via parameters in Kitchen/Busser
# See https://github.com/test-kitchen/test-kitchen/issues/662 for reference

def get_agent_flavor
  if os == :windows
    dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
  else
    dna_json_path = "/tmp/kitchen/dna.json"
  end
  JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('agent_flavor')
end

shared_examples_for 'IoT Agent install' do
  it_behaves_like 'an installed Agent'
end

shared_examples_for 'IoT Agent behavior' do
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'an Agent that stops'
  it_behaves_like 'an Agent that restarts'
end

shared_examples_for 'IoT Agent uninstall' do
  it_behaves_like 'an IoT Agent that is removed'
end

shared_examples_for 'an IoT Agent that is removed' do
  it 'should remove the agent' do
    if os == :windows
      # uninstallcmd = "start /wait msiexec /q /x 'C:\\Users\\azure\\AppData\\Local\\Temp\\kitchen\\cache\\ddagent-cli.msi'"
      uninstallcmd='for /f "usebackq" %n IN (`wmic product where "name like \'datadog%\'" get IdentifyingNumber ^| find "{"`) do start /wait msiexec /log c:\\uninst.log /q /x %n'
      expect(system(uninstallcmd)).to be_truthy
    else
      if system('which apt-get &> /dev/null')
        expect(system("sudo apt-get -q -y remove datadog-iot-agent > /dev/null")).to be_truthy
      elsif system('which yum &> /dev/null')
        expect(system("sudo yum -y remove datadog-iot-agent > /dev/null")).to be_truthy
      elsif system('which zypper &> /dev/null')
        expect(system("sudo zypper --non-interactive remove datadog-iot-agent > /dev/null")).to be_truthy
      else
        raise 'Unknown package manager'
      end
    end
  end

  it 'should not be running the agent after removal' do
    sleep 5
    expect(agent_processes_running?).to be_falsey
  end

  it 'should remove the installation directory' do
    if os == :windows
      expect(File).not_to exist("C:\\Program Files\\Datadog\\Datadog Agent\\")
    else
      expect(File).not_to exist("/opt/datadog-agent/")
    end
  end

  if os != :windows
    it 'should remove the agent link from bin' do
      expect(File).not_to exist('/usr/bin/datadog-agent')
    end
  end
end
