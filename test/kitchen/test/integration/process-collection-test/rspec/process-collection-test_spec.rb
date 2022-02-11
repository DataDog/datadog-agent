require 'spec_helper'

shared_examples_for 'a running Process Agent' do
    it 'is running' do
        if os == :windows
            expect(is_process_running?("process-agent.exe")).to be_truthy
        else
            expect(is_process_running?("process-agent")).to be_truthy
        end
    end
end

describe 'a Process Agent with Container Collection enabled' do
    it_behaves_like 'a running Process Agent'
    it 'has container collection enabled' do
        conf = read_conf_file()
        expect(conf).to have_key("process_config")
        expect(conf["process_config"]).to have_key("container_collection")
        expect(conf["process_config"]["container_collection"]).to have_key("enabled")
        expect(conf["process_config"]["container_collection"]["enabled"]).to be_truthy

    end
    it 'is running the container check' do
        expect(check_enabled?("container")).to be_truthy
    end
end
