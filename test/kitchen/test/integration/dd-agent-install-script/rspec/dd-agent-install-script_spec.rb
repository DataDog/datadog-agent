require 'spec_helper'
require 'iot_spec_helper'

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
end

describe 'dd-agent-installation-script' do
  agent_flavor = get_agent_flavor
  if agent_flavor == "datadog-agent"
    include_examples 'Agent install'
    include_examples 'Agent behavior'
    include_examples 'Agent installed by the install script'
    include_examples 'Agent uninstall'
  elsif agent_flavor == "datadog-iot-agent"
    include_examples 'IoT Agent install'
    include_examples 'IoT Agent behavior'
    include_examples 'Agent installed by the install script'
    include_examples 'IoT Agent uninstall'
  end
end
