require 'spec_helper'

describe 'dd-agent-installation-script' do
  include_examples 'Agent'

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
