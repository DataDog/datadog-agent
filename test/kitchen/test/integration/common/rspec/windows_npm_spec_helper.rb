shared_examples_for 'a Windows Agent with NPM driver that can start' do
  it 'has system probe service installed' do
    expect(is_windows_service_installed("datadog-system-probe")).to be_truthy
  end
  it 'has Windows NPM driver installed' do
    expect(is_windows_service_installed("ddnpm")).to be_truthy
  end
  it 'has Windws NPM driver files installed' do
    expect(File).to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.cat")
    expect(File).to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.sys")
    expect(File).to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.inf")
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
shared_examples_for 'a Windows Agent with no NPM driver installed' do
  it 'does not have the Windows NPM driver installed' do
    expect(is_windows_service_installed("ddnpm")).to be_falsey
  end
  it 'does not have the Windows NPM driver files installed' do
    expect(File).not_to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.cat")
    expect(File).not_to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.sys")
    expect(File).not_to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.inf")
  end
end

shared_examples_for 'a Windows Agent with NPM driver installed' do
  it 'has system probe service installed' do
    expect(is_windows_service_installed("datadog-system-probe")).to be_truthy
  end
  it 'has Windows NPM driver installed' do
    expect(is_windows_service_installed("ddnpm")).to be_truthy
  end
  it 'has Windws NPM driver files installed' do
    expect(File).to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.cat")
    expect(File).to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.sys")
    expect(File).to exist("#{ENV['ProgramFiles']}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddnpm.inf")
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
    confYaml["process_config"]["enabled"] = "true"
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
    restart "datadog-agent"
    expect(is_service_running?("datadogagent")).to be_truthy
    expect(is_service_running?("datadog-system-probe")).to be_truthy
  end
end