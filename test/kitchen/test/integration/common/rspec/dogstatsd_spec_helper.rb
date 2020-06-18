def wait_until_stopped(timeout = 60)
    # Check if the agent has stopped every second
    # Timeout after the given number of seconds
    for _ in 1..timeout do
        break if !is_running?
        sleep 1
    end
    # HACK: somewhere between 6.15.0 and 6.16.0, the delay between the
    # Agent start and the moment when the status command starts working
    # has dramatically increased.
    # Before (on ubuntu/debian):
    # - during the first ~0.05s: connection refused
    # - after: works correctly
    # Now:
    # - during the first ~0.05s: connection refused
    # - between ~0.05s and ~1s: EOF
    # - after: works correctly
    # Until we understand and fix the problem, we're adding this sleep
    # so that we don't get flakes in the kitchen tests.
    sleep 2
end

def wait_until_started(timeout = 30)
    # Check if the agent has started every second
    # Timeout after the given number of seconds
    for _ in 1..timeout do
        break if is_running?
        sleep 1
    end
    # HACK: somewhere between 6.15.0 and 6.16.0, the delay between the
    # Agent start and the moment when the status command starts working
    # has dramatically increased.
    # Before (on ubuntu/debian):
    # - during the first ~0.05s: connection refused
    # - after: works correctly
    # Now:
    # - during the first ~0.05s: connection refused
    # - between ~0.05s and ~1s: EOF
    # - after: works correctly
    # Until we understand and fix the problem, we're adding this sleep
    # so that we don't get flakes in the kitchen tests.
    sleep 5
end

def stop
    if os == :windows
        # forces the trace agent (and other dependent services) to stop
        result = system 'net stop /y dogstatsd 2>&1'
        sleep 5
    else
        if has_systemctl
        result = system 'sudo systemctl stop datadog-dogstatsd.service'
        elsif has_upstart
        result = system 'sudo initctl stop datadog-dogstatsd'
        else
        result = system "sudo /sbin/service datadog-dogstatsd stop"
        end
    end
    wait_until_stopped
    result
end

def start
    if os == :windows
        result = system 'net start dogstatsd 2>&1'
        sleep 5
    else
        if has_systemctl
        result = system 'sudo systemctl start datadog-dogstatsd.service'
        elsif has_upstart
        result = system 'sudo initctl start datadog-dogstatsd'
        else
        result = system "sudo /sbin/service datadog-dogstatsd start"
        end
    end
    wait_until_started
    result
end

def restart
    if os == :windows
        # forces the trace agent (and other dependent services) to stop
        if is_running?
        result = system 'net stop /y dogstatsd 2>&1'
        sleep 5
        wait_until_stopped
        end
        result = system 'net start dogstatsd 2>&1'
        sleep 5
        wait_until_started
    else
        if has_systemctl
            result = system 'sudo systemctl restart datadog-dogstatsd.service'
            # Worst case: the Agent has already stopped and restarted when we check if the process has been stopped
            # and we lose 5 seconds.
            wait_until_stopped 5
            wait_until_started 5
        elsif has_upstart
            # initctl can't restart
            result = system '(sudo initctl restart datadog-dogstatsd || sudo initctl start datadog-dogstatsd)'
            wait_until_stopped 5
            wait_until_started 5
        else
            result = system "sudo /sbin/service datadog-dogstatsd restart"
            wait_until_stopped 5
            wait_until_started 5
        end
    end
    result
end

def has_systemctl
    system('command -v systemctl 2>&1 > /dev/null')
end

def has_upstart
    system('/sbin/init --version 2>&1 | grep -q upstart >/dev/null')
end

def is_service_running?(svcname)
    if os == :windows
        `sc interrogate #{svcname} 2>&1`.include?('RUNNING')
    else
        if has_systemctl
            system("sudo systemctl status --no-pager #{svcname}.service")
        elsif has_upstart
            status = `sudo initctl status #{svcname}`
            status.include?('start/running')
        else
            status = `sudo /sbin/service #{svcname} status`
            status.include?('running')
        end
    end
end

def is_running?
    if os == :windows
        return is_service_running?("dogstatsd")
    else
        return is_service_running?("datadog-dogstatsd")
    end
end

def is_process_running?(pname)
    if os == :windows
        tasklist = `tasklist /fi \"ImageName eq #{pname}\" 2>&1`
        if tasklist.include?(pname)
        return true
        end
    else
        return true if system("pgrep -f #{pname}")
    end
    return false
end

def dogstatsd_processes_running?
    %w(dogstatsd dogstatsd.exe).each do |p|
        return true if is_process_running?(p)
    end
    false
end

shared_examples_for 'Dogstatsd install' do
    it_behaves_like 'an installed Dogstatsd'
end

shared_examples_for 'Dogstatsd behavior' do
    it_behaves_like 'a running Dogstatsd with no errors'
    it_behaves_like 'an Dogstatsd that stops'
    it_behaves_like 'an Dogstatsd that restarts'
end

shared_examples_for 'Dogstatsd uninstall' do
    it_behaves_like 'a Dogstatsd that is removed'
end

shared_examples_for "an installed Dogstatsd" do
    wait_until_started
  
    it 'has an example config file' do
        if os != :windows
            expect(File).to exist('/etc/datadog-agent/dogstatsd.yaml.example')
        end
    end
  
    it 'has a datadog-agent binary in usr/bin' do
      if os != :windows
        expect(File).to exist('/usr/bin/dogstatsd')
      end
    end
end

shared_examples_for "a running Dogstatsd" do
    it 'is running' do
      expect(dogstatsd_processes_running?).to be_truthy
    end
  
    it 'has a config file' do
      if os == :windows
        conf_path = "#{ENV['ProgramData']}\\Datadog\\dogstatsd.yaml"
      else
        conf_path = '/etc/datadog-agent/dogstatsd.yaml'
      end
      expect(File).to exist(conf_path)
    end
end

shared_examples_for 'a Dogstatsd that stops' do
    it 'stops' do
        output = stop
        if os != :windows
            expect(output).to be_truthy
        end
        expect(is_running?).to be_falsey
    end
  
    it 'is not running any dogstatsd processes' do
        expect(dogstatsd_processes_running?).to be_falsey
    end
  
    it 'starts after being stopped' do
        output = start
        if os != :windows
            expect(output).to be_truthy
        end
        expect(is_running?).to be_truthy
    end
  end
  
  shared_examples_for 'a Dogstatsd that restarts' do
    it 'restarts when dogstatsd is running' do
        if !is_running?
            start
        end
        output = restart
        if os != :windows
            expect(output).to be_truthy
        end
        expect(is_running?).to be_truthy
    end
  
    it 'restarts when dogstatsd is not running' do
        if is_running?
            stop
        end
        output = restart
        if os != :windows
            expect(output).to be_truthy
        end
            expect(is_running?).to be_truthy
    end
end

shared_examples_for 'a Dogstatsd that is removed' do
    it 'should remove Dogstatsd' do
        if os == :windows
        # uninstallcmd = "start /wait msiexec /q /x 'C:\\Users\\azure\\AppData\\Local\\Temp\\kitchen\\cache\\ddagent-cli.msi'"
        uninstallcmd='for /f "usebackq" %n IN (`wmic product where "name like \'datadog%\'" get IdentifyingNumber ^| find "{"`) do start /wait msiexec /log c:\\uninst.log /q /x %n'
        expect(system(uninstallcmd)).to be_truthy
        else
            if system('which apt-get &> /dev/null')
                expect(system("sudo apt-get -q -y remove datadog-dogstatsd > /dev/null")).to be_truthy
            elsif system('which yum &> /dev/null')
                expect(system("sudo yum -y remove datadog-dogstatsd > /dev/null")).to be_truthy
            elsif system('which zypper &> /dev/null')
                expect(system("sudo zypper --non-interactive remove datadog-dogstatsd > /dev/null")).to be_truthy
            else
                raise 'Unknown package manager'
            end
        end
    end

    it 'should not be running after removal' do
        sleep 5
        expect(dogstatsd_processes_running?).to be_falsey
    end

    it 'should remove the installation directory' do
        if os == :windows
        expect(File).not_to exist("C:\\Program Files\\Datadog\\Datadog Dogstatsd\\")
        else
        expect(File).not_to exist("/opt/datadog-dogstatsd/")
        end
    end

    if os != :windows
        it 'should remove the dogstatsd link from bin' do
        expect(File).not_to exist('/usr/bin/dogstatsd')
        end
    end
end

