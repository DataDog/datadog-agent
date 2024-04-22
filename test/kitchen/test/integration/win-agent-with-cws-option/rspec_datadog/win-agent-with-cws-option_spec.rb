require 'spec_helper'
require 'windows_npm_spec_helper' # for is_windows_service_disabled


shared_examples_for 'a Windows Agent with CWS driver disabled' do
    if expect_windows_cws?
        it 'has the service disabled' do
            expect(is_windows_service_disabled("ddprocmon")).to be_truthy
        end 
    end
end
  
shared_examples_for 'a Windows Agent with CWS driver installed' do
    it 'has system probe service installed' do
        expect(is_windows_service_installed("datadog-system-probe")).to be_truthy
    end

    it 'has required services installed' do
        expect(is_windows_service_installed("datadog-security-agent")).to be_truthy
        expect(is_windows_service_installed("ddprocmon")).to be_truthy
    end
    it 'has driver files' do
        program_files = safe_program_files
        expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddprocmon.cat")
        expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddprocmon.sys")
        expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\driver\\ddprocmon.inf")
    end

    it 'does not have the driver running on install' do
        ## verify that the driver is not started yet
        expect(is_service_running?("ddprocmon")).to be_falsey
    end    

end
  
shared_examples_for 'a Windows Agent with CWS running' do
    if expect_windows_cws?
        it 'has cws services not started by default' do
            expect(is_service_running?("datadog-system-probe")).to be_falsey
            expect(is_service_running?("datadog-security-agent")).to be_falsey
        end

        it 'has default config files' do
            expect(File).to exist(get_conf_file("system-probe.yaml"))
            expect(File).to exist(get_conf_file("security-agent.yaml"))
        end
        it 'can start security agent' do

            enable_cws(get_conf_file("system-probe.yaml"), true)
            enable_cws(get_conf_file("security-agent.yaml"), true)

            stop "datadog-agent"
            
            start "datadog-agent"
            sleep 30
            expect(is_service_running?("datadogagent")).to be_truthy
            expect(is_service_running?("datadog-system-probe")).to be_truthy
            expect(is_service_running?("datadog-security-agent")).to be_truthy
        end
        it 'can gracefully shut down security agent' do
            stop "datadog-agent"
            
            ## these tests return false for any state other than running.  So "shutting down"
            ## will erroneously pass here
            expect(is_service_running?("datadogagent")).to be_falsey
            expect(is_service_running?("datadog-system-probe")).to be_falsey
            expect(is_service_running?("datadog-security-agent")).to be_falsey

            ## so also check that the process is actually gone
            expect(security_agent_running?).to be_falsey
            expect(system_probe_running?).to be_falsey

        end
    end  ## endif expect CWS, no tests at all if not expected.
end
  

describe 'the agent installed with the cws component' do
    it_behaves_like 'an installed Agent'
    it_behaves_like 'a running Agent with no errors'
    it_behaves_like 'a Windows Agent with CWS driver installed'
    it_behaves_like 'a Windows Agent with CWS driver disabled'
    it_behaves_like 'a Windows Agent with CWS running'
end

