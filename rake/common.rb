require 'rake/clean'

def os
  case RUBY_PLATFORM
  when /linux/
    "linux"
  when /darwin/
    "darwin"
  when /x64-mingw32/
    "windows"
  else
    fail 'Unsupported OS'
  end
end

# `tags` option
def go_build_tags
  build_tags = ENV['tags'] || "zlib snmp etcd zk cpython jmx apm docker ec2 gce process"
  build_tags = ENV['puppy'] == 'true' ? 'zlib' : build_tags
end

def get_base_ldflags()
  # get agent-payload version
  agent_payload_version = get_payload_version()
  commit = `git rev-parse --short HEAD`.strip

  ldflags = [
    "-X #{REPO_PATH}/pkg/version.commit=#{commit}",
    "-X #{REPO_PATH}/pkg/serializer.AgentPayloadVersion=#{agent_payload_version}",
  ]
end

def go_fmt(path, fail_on_mod)
  out = `go fmt #{path}/...`
  errors = out.split("\n")
  if errors.length > 0
    puts "Reformatted the following files:"
    puts out
    fail if fail_on_mod
  end
end

def go_lint(path)
  out = `golint #{path}/...`
  errors = out.split("\n")
  puts "#{errors.length} linting issues found in #{path}"
  if errors.length > 0
    puts out
    fail
  end
end

def go_vet(path)
  out = `go vet #{path}/... 2>&1`
  errors = out.split("\n")
  puts "vet found #{errors.length} issues in #{path}"
  if errors.length > 0
    puts out
    fail
  end
end

def bin_name(name)
  case os
  when "windows"
    "#{name}.exe"
  else
    name
  end
end

# extract the agent payload version from `Gopkg.toml` without requiring an
# external package
def get_payload_version()

  current = {}

  # parse the TOML file line by line
  File.readlines("Gopkg.lock").each do |line|
    # skip empty lines and comments
    if line.length == 0 || line.start_with?("#")
      next
    end

    # change the parser "state" when we find a [[projects]] section
    if line.include? "[[projects]]"
      # see if the current section is what we're searching for
      if current.fetch('name', nil) == "github.com/DataDog/agent-payload"
        return current["version"]
      end

      # if not, reset the "state" and proceed with the next line
      current = {}
      next
    end

    # search for an assignment, ignore subsequent `=` chars
    toks = line.split('=', 2)
    if toks.length == 2
      # strip whitespaces
      key = toks.first.strip
      # strip whitespaces and quotes
      value = toks.last.tr('"', '').strip
      current[key] = value
    end
  end

  return ""
end
