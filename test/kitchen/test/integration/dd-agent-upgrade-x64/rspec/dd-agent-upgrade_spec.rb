require 'spec_helper'

describe 'the upgraded agent' do
  include_examples 'Agent install'
  include_examples 'Agent behavior'

  # We retrieve the value defined in kitchen.yml because there is no simple way
  # to set env variables on the target machine or via parameters in Kitchen/Busser
  # See https://github.com/test-kitchen/test-kitchen/issues/662 for reference
  let(:agent_expected_version) { JSON.parse(IO.read("/tmp/kitchen/dna.json")).fetch('dd-agent-upgrade-rspec').fetch('agent_expected_version') }

  it 'runs with the expected version (based on the `info` command output)' do
    agent_short_version = /(\.?\d)+/.match(agent_expected_version)[0]
    expect(info).to include "v #{agent_short_version}"
  end

  it 'runs with the expected version (based on the version manifest file)' do
    version_manifest_file = '/opt/datadog-agent/version-manifest.txt'
    expect(File).to exist(version_manifest_file)
    # Match the first line of the manifest file
    expect(File.open(version_manifest_file) {|f| f.readline.strip}).to match "datadog-agent #{agent_expected_version}"
  end

  include_examples 'Agent uninstall'
end
