require "./lib/ostools.rb"

name "datadog-trace-agent"

default_version "5.17.1"

source git: 'https://github.com/DataDog/datadog-trace-agent.git'

if windows?
  trace_agent_binary = "trace-agent.exe"
else
  trace_agent_binary = "trace-agent"
end

gopath = ENV['GOPATH']
gopath_bin = File.join(gopath, "bin")
datadog_gopath = File.join(gopath, "src", "github.com", "DataDog")
agent_source = File.join(Omnibus::Config.source_dir, "datadog-trace-agent")
agent_gopath = File.join(datadog_gopath, "datadog-trace-agent")
glide_gopath = File.join(gopath, "src", "github.com", "Masterminds", "glide")

env = {
  "GOPATH" => gopath,
  "PATH" => "#{gopath_bin}:#{ENV["PATH"]}",
  "TRACE_AGENT_VERSION" => default_version, # used by gorake.rb in the trace-agent
}

build do
   ship_license "https://raw.githubusercontent.com/DataDog/datadog-trace-agent/#{version}/LICENSE"

   # Put datadog-trace-agent into the current GOPATH in the right src path
   sync agent_source, agent_gopath

   # Checkout glide without installing it
   command "go get -d -u github.com/Masterminds/glide", :env => env
   # Pin glide to a known version
   command "git checkout v0.12.3", :env => env, :cwd => glide_gopath
   # Finally install glide
   command "go install github.com/Masterminds/glide", :env => env

   # Build datadog-trace-agent
   command "glide install", :env => env, :cwd => agent_gopath
   command "rake build", :env => env, :cwd => agent_gopath
   command "mv ./#{trace_agent_binary} #{install_dir}/embedded/bin/", :env => env, :cwd => agent_gopath
end
