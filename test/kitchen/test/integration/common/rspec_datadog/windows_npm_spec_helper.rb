require 'win32/registry'

def is_windows_service_disabled(service)
  keypath = "SYSTEM\\CurrentControlSet\\Services\\#{service}"
  type = 0;
  Win32::Registry::HKEY_LOCAL_MACHINE.open(keypath) do |reg|
    type = reg['Start']
  end
  return true if type == 4
  return false
end 
shared_examples_for 'a Windows Agent with NPM driver that can start' do
  it 'has system probe service installed' do
    expect(is_windows_service_installed("datadog-system-probe")).to be_truthy
  end
  it 'has Windows NPM driver installed' do
    expect(is_windows_service_installed("ddnpm")).to be_truthy
  end
  it 'has Windows NPM driver files installed' do
    program_files = safe_program_files
    expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.cat")
    expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.sys")
    expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.inf")
  end

  it 'does not have the driver running on install' do
    ## verify that the driver is not started yet
    expect(is_service_running?("ddnpm")).to be_falsey
  end


  it 'can successfully start the driver' do
    ## start the service
    result = system "net start ddnpm 2>&1"

    ## now expect it to be running
    expect(is_service_running?("ddnpm")).to be_truthy
  end

end
shared_examples_for 'a Windows Agent with NPM driver disabled' do
  it 'has the service disabled' do
    expect(is_windows_service_disabled("ddnpm")).to be_truthy
  end 
end

shared_examples_for 'a Windows Agent with NPM driver installed' do
  it 'has system probe service installed' do
    expect(is_windows_service_installed("datadog-system-probe")).to be_truthy
  end
  it 'has Windows NPM driver installed' do
    expect(is_windows_service_installed("ddnpm")).to be_truthy
  end
  it 'has Windows NPM driver files installed' do
    program_files = safe_program_files
    expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.cat")
    expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.sys")
    expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.inf")
  end
end

shared_examples_for 'a Windows Agent with NPM running' do
  it 'can start system probe' do
    conf_path = ""
    if os != :windows
      conf_path = "/etc/datadog-agent/datadog.yaml"
    else
      conf_path = "#{ENV['ProgramData']}\\Datadog\\datadog.yaml"
    end
    f = File.read(conf_path)
    confYaml = YAML.load(f)
    if !confYaml.key("process_config")
      confYaml["process_config"] = {}
    end
    confYaml["process_config"]["process_collection"] = { "enabled": true }
    File.write(conf_path, confYaml.to_yaml)

    if os != :windows
      spconf_path = "/etc/datadog-agent/datadog.yaml"
    else
      spconf_path = "#{ENV['ProgramData']}\\Datadog\\system-probe.yaml"
    end
    spf = File.read(spconf_path)
    spconfYaml = YAML.load(spf)
    if !spconfYaml
      spconfYaml = {}
    end
    if !spconfYaml.key("network_config")
      spconfYaml["network_config"] = {}
    end
    spconfYaml["network_config"]["enabled"] = true
    File.write(spconf_path, spconfYaml.to_yaml)

    expect(is_service_running?("datadog-system-probe")).to be_falsey
    #restart "datadog-agent"
    stop "datadog-agent"
    sleep 10
    start "datadog-agent"
    sleep 20
    expect(is_service_running?("datadogagent")).to be_truthy
    expect(is_service_running?("datadog-system-probe")).to be_truthy
  end
end

shared_examples_for 'a Windows Agent with closed source enabled' do
  ena = 0
  Win32::Registry::HKEY_LOCAL_MACHINE.open('SOFTWARE\DataDog\Datadog Agent') do |reg|
    ena = reg['AllowClosedSource']
  end
  it 'registry value is set to one' do
    expect(ena).to eq(1)
  end

end

shared_examples_for 'a Windows Agent with closed source disabled' do
  ena = 0
  Win32::Registry::HKEY_LOCAL_MACHINE.open('SOFTWARE\DataDog\Datadog Agent') do |reg|
    ena = reg['AllowClosedSource']
  end
  it 'registry value is set to zero' do
    expect(ena).to eq(0)
  end

end