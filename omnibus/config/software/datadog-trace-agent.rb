name "datadog-trace-agent"
source git: 'https://github.com/DataDog/datadog-trace-agent.git'

require "./lib/ostools.rb"

trace_agent_branch = ENV['TRACE_AGENT_BRANCH']
if trace_agent_branch.nil? || trace_agent_branch.empty?
  trace_agent_branch = 'master'
end
default_version trace_agent_branch

trace_agent_add_build_vars = true
if ENV.has_key?('TRACE_AGENT_ADD_BUILD_VARS') && ENV['TRACE_AGENT_ADD_BUILD_VARS'] == 'false'
  trace_agent_add_build_vars = false
end

if windows?
  trace_agent_binary = "trace-agent.exe"
else
  trace_agent_binary = "trace-agent"
end
dd_agent_version = ENV['AGENT_VERSION']
gopath = ENV['GOPATH']

agent_source_dir = File.join(Omnibus::Config.source_dir, "datadog-trace-agent")
glide_cache_dir = "#{gopath}/src/github.com/Masterminds/glide"
agent_cache_dir = "#{gopath}/src/github.com/DataDog/datadog-trace-agent"

env = {
  "GOPATH" => gopath,
  "PATH" => "#{gopath}/bin:#{ENV["PATH"]}",
  "TRACE_AGENT_VERSION" => dd_agent_version, # used by gorake.rb in the trace-agent
  "TRACE_AGENT_ADD_BUILD_VARS" => trace_agent_add_build_vars.to_s(),
}

build do
   ship_license "https://raw.githubusercontent.com/DataDog/datadog-trace-agent/#{version}/LICENSE"

   # Put datadog-trace-agent into a valid GOPATH
   puts "source dir #{agent_source_dir}"
   mkdir "#{gopath}/src/github.com/DataDog/"
   delete File.join(gopath, "src", "github.com", "DataDog", "datadog-trace-agent")
   move agent_source_dir, "#{gopath}/src/github.com/DataDog/"

   # Checkout datadog-trace-agent's build dependencies
   command "go get -d github.com/Masterminds/glide", :env => env, :cwd => agent_cache_dir

   # Pin build deps to known versions
   command "git reset --hard v0.12.3", :env => env, :cwd => glide_cache_dir
   command "go install github.com/Masterminds/glide", :env => env, :cwd => glide_cache_dir

   # Build datadog-trace-agent
   command "#{gopath}/bin/glide install", :env => env, :cwd => agent_cache_dir
   command "rake build", :env => env, :cwd => agent_cache_dir
   command "mv ./#{trace_agent_binary} #{install_dir}/embedded/bin/", :env => env, :cwd => agent_cache_dir
end
