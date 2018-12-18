require_relative 'spec_helper'


shared_examples_for 'an Agent with APM enabled' do
  it 'has apm enabled' do
    confYaml = read_conf_file()
    expect(confYaml).to have_key("apm_config")
    expect(confYaml["apm_config"]).to have_key("enabled")
    expect(confYaml["apm_config"]["enabled"]).to be_truthy
    expect(is_port_bound(8126)).to be_truthy
  end
  it 'has the apm agent running' do
    expect(is_process_running?("trace-agent.exe")).to be_truthy
    expect(is_service_running?("datadog-trace-agent")).to be_truthy
  end
end

shared_examples_for 'an Agent with logs enabled' do
  it 'has logs enabled' do
    confYaml = read_conf_file()
    expect(confYaml).to have_key("logs_config")
    expect(confYaml).to have_key("logs_enabled")
    expect(confYaml["logs_enabled"]).to be_truthy
  end
end

shared_examples_for 'an Agent with process enabled' do
  it 'has process enabled' do
    confYaml = read_conf_file()
    expect(confYaml).to have_key("process_config")
    expect(confYaml["process_config"]).to have_key("enabled")
    expect(confYaml["process_config"]["enabled"]).to be_truthy
  end
  it 'has the process agent running' do
    expect(is_process_running?("process-agent.exe")).to be_truthy
    expect(is_service_running?("datadog-process-agent")).to be_truthy
  end
end

describe 'dd-agent-all-subservices' do
  it_behaves_like 'an installed Agent'
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'an Agent with APM enabled'
  it_behaves_like 'an Agent with logs enabled'
  it_behaves_like 'an Agent with process enabled'
end
  