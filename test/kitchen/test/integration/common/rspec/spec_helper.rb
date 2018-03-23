require 'json'
require 'open-uri'
require 'rspec'
require 'rbconfig'

os_cache = nil

def os
  # OS Detection from https://stackoverflow.com/questions/11784109/detecting-operating-systems-in-ruby
  os_cache ||= (
    host_os = RbConfig::CONFIG['host_os']
    case host_os
    when /mswin|msys|mingw|cygwin|bccwin|wince|emc/
      :windows
    when /darwin|mac os/
      :macosx
    when /linux/
      :linux
    when /solaris|bsd/
      :unix
    else
      raise Error::WebDriverError, "unknown os: #{host_os.inspect}"
    end
  )
end

def agent_command
  if os == :windows
    '"C:\\Program Files\\Datadog\\Datadog Agent\\embedded\\agent.exe"'
  else
    "sudo datadog-agent"
  end
end

def stop
  if os == :windows
    # forces the trace agent (and other dependent services) to stop
    system 'net stop /y datadogagent 2>&1'
    sleep 15
  else
    if has_systemctl
      system 'sudo systemctl stop datadog-agent.service && sleep 10'
    else
      system 'sudo initctl stop datadog-agent && sleep 10'
    end
  end
end

def start
  if os == :windows
    system 'net start datadogagent 2>&1'
    sleep 15
  else
    if has_systemctl
      system 'sudo systemctl start datadog-agent.service && sleep 10'
    else
      system 'sudo initctl start datadog-agent && sleep 10'
    end
  end
end

def restart
  if os == :windows
    # forces the trace agent (and other dependent services) to stop
    if is_running?
      system 'net stop /y datadogagent 2>&1'
      sleep 15
    end
    system 'net start datadogagent 2>&1'
    sleep 15
  else
    if has_systemctl
      system 'sudo systemctl restart datadog-agent.service && sleep 10'
    else
      # initctl can't restart
      system '(sudo initctl restart datadog-agent || sudo initctl start datadog-agent) && sleep 10'
    end
  end
end

def has_systemctl
  system('command -v systemctl 2>&1 > /dev/null')
end

def info
  `#{agent_command} status 2>&1`
end

def json_info
  info_output = `#{agent_command} status -j 2>&1`
  info_output = info_output.gsub("Getting the status from the agent.", "")

  # removes any stray log lines
  info_output = info_output.gsub(/[0-9]+[ ]\[[a-zA-Z]+\][a-zA-Z \t%:\\]+$/, "")

  JSON.parse(info_output)
end

def status
  if os == :windows
    `sc interrogate datadogagent 2>&1`.include?('RUNNING')
  else
    if has_systemctl
      system('sudo systemctl status --no-pager datadog-agent.service')
    else
      system('sudo initctl status datadog-agent')
    end
  end
end

def is_running?
  if os == :windows
    `sc interrogate datadogagent 2>&1`.include?('RUNNING')
  else
    if has_systemctl
      system('sudo systemctl status --no-pager datadog-agent.service')
    else
      status = `sudo initctl status datadog-agent`
      status.include?('start/running')
    end
  end
end

def agent_processes_running?
  %w(datadog-agent agent.exe).each do |p|
    if os == :windows
      tasklist = `tasklist /fi \"ImageName eq #{p}\" 2>&1`
      if tasklist.include?(p)
        return true
      end
    else
      return true if system("pgrep -f #{p}")
    end
  end
  false
end

def read_agent_file(path, commit_hash)
  open("https://raw.githubusercontent.com/DataDog/datadog-agent/#{commit_hash}/#{path}").read()
end

# Hash of the commit the Agent was built from
def agent_git_hash
  JSON.parse(IO.read("/opt/datadog-agent/version-manifest.json"))['software']['datadog-agent']['locked_version']
end

def trace_agent_git_hash
  JSON.parse(IO.read("/opt/datadog-agent/version-manifest.json"))['software']['datadog-trace-agent']['locked_version']
end

# From a pip-requirements-formatted string, return a hash of 'dep_name' => 'version'
def read_requirements(file_contents)
  reqs = Hash.new
  file_contents.lines.reject do |line|
    /^#/ === line  # reject comment lines
  end.collect do |line|
    /(.+)==([^\s]+)/.match(line)
  end.compact.each do |match|
    reqs[match[1].downcase] = match[2]
  end
  reqs
end

def pip_freeze
  `/opt/datadog-agent/embedded/bin/pip freeze 2> /dev/null`
end

def is_port_bound(port)
  if os == :windows
    port_regex = Regexp.new(port.to_s)
    port_regex.match(`netstat -n -b -a -p TCP 2>&1`)
  else
    system("sudo netstat -lntp | grep #{port} 1>/dev/null")
  end
end

shared_examples_for 'Agent' do
  it_behaves_like 'an installed Agent'
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'an Agent that stops'
  it_behaves_like 'an Agent that restarts'
  it_behaves_like 'an Agent that is removed'
end

shared_examples_for "an installed Agent" do
  it 'has an example config file' do
    if os != :windows
      expect(File).to exist('/etc/datadog-agent/datadog.yaml.example')
    end
  end

  it 'has a datadog-agent binary in usr/bin' do
    if os != :windows
      expect(File).to exist('/usr/bin/datadog-agent')
    end
  end

  it 'is properly signed' do
    if os == :windows
      # The user in the yaml file is "datadog", however the default test kitchen user is azure.
      # This allows either to be used without changing the test.
      msi_path_base = 'C:\\Users\\'
      msi_path_end = '\\AppData\\Local\\Temp\\kitchen\\cache\\ddagent-cli.msi'
      msi_path_azure = msi_path_base + 'azure' + msi_path_end
      msi_path_datadog = msi_path_base + 'datadog' + msi_path_end
      if File.exist?(msi_path_azure)
        msi_path = msi_path_azure
      else
        msi_path = msi_path_datadog
      end
      output = `powershell -command "get-authenticodesignature #{msi_path}"`
      signature_hash = "ECCDAE36FDCB654D2CBAB3E8975AA55469F96E4C"
      expect(output).to include(signature_hash)
      expect(output).to include("Valid")
      expect(output).not_to include("NotSigned")
    end
  end
end

shared_examples_for "a running Agent with no errors" do
  it 'has an agent binary' do
    if os != :windows
      expect(File).to exist('/usr/bin/datadog-agent')
    end
  end

  it 'is running' do
    expect(status).to be_truthy
  end

  it 'has a config file' do
    if os == :windows
      conf_path = "#{ENV['ProgramData']}\\Datadog\\datadog.yaml"
    else
      conf_path = '/etc/datadog-agent/datadog.yaml'
    end
    expect(File).to exist(conf_path)
  end

  # it 'has running checks' do


  #   # On systems that use systemd (on which the `start` script returns immediately)
  #   # sleep a few seconds to let the collector finish its first run
  #   # This seems to happen on windows, too
  #   if os != :windows
  #     system('command -v systemctl 2>&1 > /dev/null && sleep 300')
  #   else
  #     sleep 300
  #   end

  #   json_info_output = json_info
  #   expect(json_info_output).to have_key("runnerStats")
  #   expect(json_info_output['runnerStats']).to have_key("Checks")
  #   expect(json_info_output['runnerStats']['Checks']).not_to be_empty
  # end

  it 'has an info command' do
    # On systems that use systemd (on which the `start` script returns immediately)
    # sleep a few seconds to let the collector finish its first run
    # Windows seems to frequently have this same issue
    if os != :windows
      system('command -v systemctl 2>&1 > /dev/null && sleep 5')
    else
      sleep 5
    end

    expect(info).to include "Forwarder"
    expect(info).to include "DogStatsD"
    expect(info).to include "Host Info"
  end

  it 'has no errors in the info command' do
    info_output = info
    # The api key is invalid. this test ensures there are no other errors
    info_output = info_output.gsub "[ERROR] API Key is invalid" "API Key is invalid"
    expect(info_output).to_not include 'ERROR'
  end

  it 'is bound to the port that receives traces by default' do
    expect(is_port_bound(8126)).to be_truthy
  end

  it 'is not bound to the port that receives traces when apm_enabled is set to false' do
    conf_path = ""
    if os != :windows
      conf_path = "/etc/datadog-agent/datadog.yaml"
    else
      conf_path = "#{ENV['ProgramData']}\\Datadog\\datadog.yaml"
    end
    open(conf_path, 'a') do |f|
      f.puts "\napm_config:\n  enabled: false"
    end
    output = restart
    if os != :windows
      expect(output).to be_truthy
      system 'command -v systemctl 2>&1 > /dev/null || sleep 5 || true'
    else
      sleep 5
    end
    expect(is_port_bound(8126)).to be_falsey
  end

  it "doesn't say 'not running' in the info command" do
    # Until it runs the logs agent by default it will say this
    # expect(info).to_not include 'not running'
  end
end

shared_examples_for 'an Agent that stops' do
  it 'stops' do
    output = stop
    if os != :windows
      expect(output).to be_truthy
    end
    expect(is_running?).to be_falsey
  end

  it 'has connection refuse in the info command' do
    if os == :windows
      expect(info).to include 'No connection could be made'
    else
      expect(info).to include 'connection refuse'
    end
  end

  it 'is not running any agent processes' do
    sleep 5 # need to wait for the Agent to stop
    expect(agent_processes_running?).to be_falsey
  end

  it 'starts after being stopped' do
    output = start
    if os != :windows
      expect(output).to be_truthy
    end
    expect(status).to be_truthy
  end
end

shared_examples_for 'an Agent that restarts' do
  it 'restarts when the agent is running' do
    if !is_running?
      start
    end
    output = restart
    if os != :windows
      expect(output).to be_truthy
    end
    expect(is_running?).to be_truthy
  end

  it 'restarts when the agent is not running' do
    if is_running?
      stop
    end
    output = restart
    if os != :windows
      expect(output).to be_truthy
    end
    expect(status).to be_truthy
  end
end

shared_examples_for 'an Agent that is removed' do
  it 'should remove the agent' do
    if os == :windows
      # uninstallcmd = "start /wait msiexec /q /x 'C:\\Users\\azure\\AppData\\Local\\Temp\\kitchen\\cache\\ddagent-cli.msi'"
      uninstallcmd='for /f "usebackq" %n IN (`wmic product where "name like \'datadog%\'" get IdentifyingNumber ^| find "{"`) do start /wait msiexec /log c:\\uninst.log /q /x %n'
      expect(system(uninstallcmd)).to be_truthy
    else
      if system('which apt-get &> /dev/null')
        expect(system("sudo apt-get -q -y remove datadog-agent > /dev/null")).to be_truthy
      elsif system('which yum &> /dev/null')
        expect(system("sudo yum -y remove datadog-agent > /dev/null")).to be_truthy
      elsif system('which zypper &> /dev/null')
        expect(system("sudo zypper --non-interactive remove datadog-agent > /dev/null")).to be_truthy
      else
        raise 'Unknown package manager'
      end
    end
  end

  it 'should not be running the agent after removal' do
    sleep 5
    expect(agent_processes_running?).to be_falsey
  end

  it 'should remove the agent binary' do
    if os != :windows
      agent_path = '/usr/bin/datadog-agent'
    else
      agent_path = "C:\\Program Files\\Datadog\\Datadog Agent\\embedded\\agent.exe"
    end
    expect(File).not_to exist(agent_path)
  end

  it 'should remove the trace-agent binary' do
    if os == :windows
      trace_agent_path = "C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\trace-agent"
    else
      trace_agent_path = '/opt/datadog-agent/bin/trace-agent'
    end
    expect(File).not_to exist(trace_agent_path)
  end
end
