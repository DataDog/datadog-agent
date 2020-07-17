require 'spec_helper'

describe 'the upgraded agent' do
  it_behaves_like 'an installed Agent'
  it_behaves_like 'a running Agent with no errors'

  # We retrieve the value defined in kitchen.yml because there is no simple way
  # to set env variables on the target machine or via parameters in Kitchen/Busser
  # See https://github.com/test-kitchen/test-kitchen/issues/662 for reference
  let(:agent_expected_version) {
    if os == :windows
      dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    else
      dna_json_path = "/tmp/kitchen/dna.json"
    end
    JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-upgrade-rspec').fetch('agent_expected_version')
  }

  it 'runs with the expected version (based on the `info` command output)' do
    agent_short_version = /(\.?\d)+/.match(agent_expected_version)[0]
    expect(info).to include "v#{agent_short_version}"
  end

  it 'runs with the expected version (based on the version manifest file)' do
    if os == :windows
      version_manifest_file = "C:/Program Files/Datadog/Datadog Agent/version-manifest.txt"
    else
      version_manifest_file = '/opt/datadog-agent/version-manifest.txt'
    end
    expect(File).to exist(version_manifest_file)
    # Match the first line of the manifest file
    expect(File.open(version_manifest_file) {|f| f.readline.strip}).to match "agent #{agent_expected_version}"
  end
end
