require_relative 'spec_helper'


shared_examples_for 'an Agent with APM disabled' do
  it 'has apm disabled' do
    confYaml = read_conf_file()
    expect(confYaml).to have_key("apm_config")
    expect(confYaml["apm_config"]).to have_key("enabled")
    expect(confYaml["apm_config"]["enabled"]).to be_falsey
    expect(is_port_bound(8126)).to be_falsey
  end
  it 'does not have the apm agent running' do
    expect(is_process_running?("trace-agent.exe")).to be_falsey
    expect(is_service_running?("datadog-trace-agent")).to be_falsey
  end
end

shared_examples_for 'an Agent with logs disabled' do
  it 'has logs disabled' do
    confYaml = read_conf_file()
    expect(confYaml).to have_key("logs_config")
    expect(confYaml).to have_key("logs_enabled")
    expect(confYaml["logs_enabled"]).to be_falsey
  end
end

shared_examples_for 'an Agent with process disabled' do
  it 'has process disabled' do
    confYaml = read_conf_file()
    expect(confYaml).to have_key("process_config")
    expect(confYaml["process_config"]).to have_key("process_collection")
    expect(confYaml["process_config"]["process_collection"]).to have_key("enabled")
    expect(confYaml["process_config"]["process_collection"]["enabled"]).to eq(false)
    expect(confYaml["process_config"]["process_discovery"]["enabled"]).to eq(false)
  end
  it 'does not have the process agent running' do
    expect(is_process_running?("process-agent.exe")).to be_falsey
    expect(is_service_running?("datadog-process-agent")).to be_falsey
  end
end

describe 'win-no-subservices' do
  it_behaves_like 'an installed Agent'
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'an Agent with APM disabled'
  it_behaves_like 'an Agent with logs disabled'
  it_behaves_like 'an Agent with process disabled'
end
