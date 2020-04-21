require 'spec_helper'

describe 'dd-agent-installation-script' do
  include_examples 'Agent'

  context 'when testing DD_SITE' do
    let(:config_file_path) do
      if os == :windows
        "#{ENV['ProgramData']}\\Datadog\\datadog.yaml"
      else
        '/etc/datadog-agent/datadog.yaml'
      end
    end

    let(:config) do
      YAML.load_file(config_file_path)
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
        'name' => 'install_script',
        'tool' => 'install_script',
        'version' => /^\d+\.\d+\.\d+$/
      )
    end
  end
end
