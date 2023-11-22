require 'spec_helper'
require 'windows_npm_spec_helper' # for is_windows_service_disabled


shared_examples_for 'a Windows Agent with CWS driver disabled' do
    # We retrieve the value defined in kitchen.yml because there is no simple way
    # to set env variables on the target machine or via parameters in Kitchen/Busser
    # See https://github.com/test-kitchen/test-kitchen/issues/662 for reference
    let(:expect_cws_installed) {
        parse_dna().fetch('dd-agent-rspec').fetch('cws_included')
    }

    it 'has the service disabled' do
        
        if expect_cws_installed
            expect(is_windows_service_disabled("ddprocmon")).to be_truthy
        end 
    end
end
  
shared_examples_for 'a Windows Agent with CWS driver installed' do
    it 'has system probe service installed' do
        expect(is_windows_service_installed("datadog-system-probe")).to be_truthy
    end

    # We retrieve the value defined in kitchen.yml because there is no simple way
    # to set env variables on the target machine or via parameters in Kitchen/Busser
    # See https://github.com/test-kitchen/test-kitchen/issues/662 for reference

    # the `let` has to be inside an it block.
    let(:expect_cws_installed) {
        parse_dna().fetch('dd-agent-rspec').fetch('cws_included')
    }

    it 'has properly installed driver files' do
        if expect_cws_installed
            expect(is_windows_service_installed("datadog-security-agent")).to be_truthy
            expect(is_windows_service_installed("ddprocmon")).to be_truthy

            program_files = safe_program_files
            expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\ddprocmon.cat")
            expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\ddprocmon.sys")
            expect(File).to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\ddprocmon.inf")
        else
            expect(is_windows_service_installed("datadog-security-agent")).to be_falsey
            expect(is_windows_service_installed("ddprocmon")).to be_falsey

            program_files = safe_program_files
            expect(File).not_to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\ddprocmon.cat")
            expect(File).not_to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\ddprocmon.sys")
            expect(File).not_to exist("#{program_files}\\DataDog\\Datadog Agent\\bin\\agent\\ddprocmon.inf")

        end
    end
    it 'does not have the driver running on install' do
        ## verify that the driver is not started yet
        expect(is_service_running?("ddprocmon")).to be_falsey
    end

end
  
shared_examples_for 'a Windows Agent with CWS running' do
    # We retrieve the value defined in kitchen.yml because there is no simple way
    # to set env variables on the target machine or via parameters in Kitchen/Busser
    # See https://github.com/test-kitchen/test-kitchen/issues/662 for reference
    let(:expect_cws_installed) {
        parse_dna().fetch('dd-agent-rspec').fetch('cws_included')
    }
    it 'can start security agent' do
        if expect_cws_installed
            sa_conf_path = ""
            sp_conf_path = ""
            if os != :windows
                sa_conf_path = "/etc/datadog-agent/security-agent.yaml"
                sp_conf_path = "/etc/datadog-agent/system-probe.yaml"
            else
                sa_conf_path = "#{ENV['ProgramData']}\\Datadog\\security-agent.yaml"
                sp_conf_path = "#{ENV['ProgramData']}\\Datadog\\system-probe.yaml"
            end
            f = File.read(sa_conf_path)
            confYaml = YAML.load(f)
            if !confYaml
                confYaml = {}
            end
            if !confYaml.key("runtime_security_config")
                confYaml["runtime_security_config"] = {}
            end
            confYaml["runtime_security_config"]["enabled"] = true
            File.write(sa_conf_path, confYaml.to_yaml)
        
            spf = File.read(sp_conf_path)
            spconfYaml = YAML.load(spf)
            if !spconfYaml
                spconfYaml = {}
            end
            if !spconfYaml.key("runtime_security_config")
                spconfYaml["runtime_security_config"] = {}
            end
            spconfYaml["runtime_security_config"]["enabled"] = true
            File.write(sp_conf_path, spconfYaml.to_yaml)
        
            expect(is_service_running?("datadog-system-probe")).to be_falsey
            expect(is_service_running?("datadog-security-agent")).to be_falsey
            #restart "datadog-agent"
            stop "datadog-agent"
            sleep 10
            start "datadog-agent"
            sleep 30
            expect(is_service_running?("datadogagent")).to be_truthy
            expect(is_service_running?("datadog-system-probe")).to be_truthy
            expect(is_service_running?("datadog-security-agent")).to be_truthy


            stop "datadog-agent"
            sleep 30
            expect(is_service_running?("datadogagent")).to be_falsey
            expect(is_service_running?("datadog-system-probe")).to be_falsey
            expect(is_service_running?("datadog-security-agent")).to be_falsey
        end
    end
end
  

describe 'the agent installed with the cws component' do
    it_behaves_like 'an installed Agent'
    it_behaves_like 'a running Agent with no errors'
    it_behaves_like 'a Windows Agent with CWS driver installed'
    it_behaves_like 'a Windows Agent with CWS driver disabled'
    it_behaves_like 'a Windows Agent with CWS running'
end

