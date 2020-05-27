require 'spec_helper'

# We retrieve the value defined in kitchen.yml because there is no simple way
# to set env variables on the target machine or via parameters in Kitchen/Busser
# See https://github.com/test-kitchen/test-kitchen/issues/662 for reference
let(:agent_flavor) {
  if os == :windows
    dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
  else
    dna_json_path = "/tmp/kitchen/dna.json"
  end
  JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('agent_flavor')
}

shared_examples_for 'Agent installed by the install script' do
  context 'when testing DD_SITE' do
    let(:config) do
      YAML.load_file('/etc/datadog-agent/datadog.yaml')
    end

    it 'uses DD_SITE to set the site' do
      expect(config['site']).to eq 'datadoghq.eu'
    end
  end

  context 'when testing the install infos' do
    let(:install_info_path) do
      '/etc/datadog-agent/install_info'
    end

    let(:install_info) do
      YAML.load_file(install_info_path)
    end

    it 'adds an install_info' do
      expect(install_info['install_method']).to match(
        'tool' => 'install_script',
        'tool_version' => 'install_script',
        'installer_version' => /^install_script-\d+\.\d+\.\d+$/
      )
    end
end

describe 'dd-agent-installation-script' do
  include_examples 'Agent install'

  if agent_flavor == "datadog-agent"
    include_examples 'Agent behavior'
    include_examples 'Agent installed by the install script'
    include_examples 'Agent uninstall'
  elsif agent_flavor == "datadog-iot-agent"
    include_examples 'IoT Agent behavior'
    include_examples 'Agent installed by the install script'
    include_examples 'IoT Agent uninstall'
  end
end
