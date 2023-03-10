require 'json'
require 'net/http'
require 'rbconfig'
require 'rspec'
require 'yaml'

#
# this enables RSpec output so that individual tests ("it behaves like...") are
# logged.
RSpec.configure do |c|
    c.default_formatter = "documentation"
end

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

def get_with_retries(uri_or_host, path, port, max_retries=10)
  retries = 0
  begin
    return Net::HTTP.get(uri_or_host, path, port)
  rescue Exception => e
    if retries < max_retries
      retries += 1
      sleep 1
      retry
    else
      raise # if we're out of retries, raise the last exception
    end
  end
end

def get_runtime_config
  res = get_with_retries('localhost', '/config', 6162)
  YAML.load(res)
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
  res = get_with_retries('localhost', '/debug/vars', 6062)
  JSON.parse(res)["enabled_checks"].include? check_name
end
