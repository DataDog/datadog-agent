require 'spec_helper'

def read_conf_file
    conf_path = ""
    if os == :windows
      conf_path = "#{ENV['ProgramData']}\\Datadog\\datadog.yaml"
      ddpath_exists = File.exist?("#{ENV['ProgramData']}\\Datadog")
      ddfile_exists = File.exist?(conf_path)
      puts "path exists #{ddpath_exists} file exists #{ddfile_exists}"
    else
      conf_path = '/etc/datadog-agent/datadog.yaml'
    end
    puts "cp is #{conf_path}"
    f = File.read(conf_path)
    confYaml = YAML.load(f)
    confYaml
end


shared_examples_for 'an Agent with APM disabled' do
  it 'has apm disabled' do
    confYaml = read_conf_file()
    expect(confYaml).to have_key("apm_config")
    expect(confYaml["apm_config"]).to have_key("enabled")
    expect(confYaml["apm_config"]["enabled"]).to be_falsey
    expect(is_port_bound(8126)).to be_falsey
  end
end

shared_examples_for 'a configured Agent' do 
    confYaml = read_conf_file()
    it 'has an API key' do
      expect(confYaml).to have_key("api_key")
      expect(confYaml["api_key"]).to be_truthy  # for now just accept that something's there
    end
    it 'has tags set' do
      expect(confYaml).to have_key("tags")
      expect(confYaml["tags"]).to include("k1:v1", "k2:v2")
      expect(confYaml["tags"]).not_to include("k1:v2")
      expect(confYaml["tags"]).not_to include("k2:v1")
    end
    it 'has CMDPORT set' do 
      expect(confYaml).to have_key("cmd_port")
      expect(confYaml["cmd_port"]).to equal(4999)
      expect(is_port_bound(4999)).to be_truthy
      expect(is_port_bound(5001)).to be_falsey
    end
    it 'has proxy settings' do 
     expect(confYaml).to have_key("proxy")
     expect(confYaml["proxy"]).to have_key("https")
     expect(confYaml["proxy"]["https"]).to eq("http://puser:ppass@proxy.foo.com:1234")
    end
    it 'has site settings' do
      expect(confYaml).to have_key("site")
      expect(confYaml["site"]).to eq("eu")

      expect(confYaml).to have_key("dd_url")
      expect(confYaml["dd_url"]).to eq("https://someurl.datadoghq.com")

      expect(confYaml).to have_key("logs_config")
      expect(confYaml["logs_config"]).to have_key("logs_dd_url")
      expect(confYaml["logs_config"]["logs_dd_url"]).to eq("https://logs.someurl.datadoghq.com")

      expect(confYaml).to have_key("process_config")
      expect(confYaml["process_config"]).to have_key("process_dd_url")
      expect(confYaml["process_config"]["process_dd_url"]).to eq("https://process.someurl.datadoghq.com")

      expect(confYaml).to have_key("apm_config")
      expect(confYaml["apm_config"]).to have_key("apm_dd_url")
      expect(confYaml["apm_config"]["apm_dd_url"]).to eq("https://trace.someurl.datadoghq.com")

    end
end


describe 'dd-agent-installopts' do
  it_behaves_like 'an installed Agent'
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'a configured Agent'
end
  