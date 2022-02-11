require 'rspec'
require 'net/http'

#
# this enables RSpec output so that individual tests ("it behaves like...") are
# logged.
RSpec.configure do |c|
    c.default_formatter = "documentation"
end

def read_conf_file
    conf_path = ""
    if os == :windows
      conf_path = "#{ENV['ProgramData']}\\Datadog\\datadog.yaml"
    else
      conf_path = '/etc/datadog-agent/datadog.yaml'
    end
    puts "cp is #{conf_path}"
    f = File.read(conf_path)
    confYaml = YAML.load(f)
    confYaml
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

def check_enabled?(check_name)
  res = Net::HTTP.get('localhost', '/debug/vars', 6062)
  JSON.parse(res)["enabled_checks"].include? check_name
end
