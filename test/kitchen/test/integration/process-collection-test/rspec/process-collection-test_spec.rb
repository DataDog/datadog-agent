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

describe 'a Process Agent with Process Collection enabled' do
    it_behaves_like 'a running Process Agent'
    it 'has process collection enabled' do
        conf = get_runtime_config()
        expect(conf).to have_key("process_collection")
        expect(conf["process_collection"]).to have_key("enabled")
        expect(conf["process_collection"]["enabled"]).to be_truthy
    end
    it 'is running the process check' do
        expect(check_enabled?("process")).to be_truthy
        expect(check_enabled?("rtprocess")).to be_truthy
    end
end
