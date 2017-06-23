require_relative './common'


def agent_bin_name
  case os
  when "windows"
    "agent.exe"
  else
    "agent.bin"
  end
end

namespace :agent do
  BIN_PATH="./bin/agent"
  CLOBBER.include(BIN_PATH)
  desc "Build the Agent [race=false|incremental=false|tags=*]"
  task :build do
    # `race` option
    race_opt = ENV['race'] == "true" ? "-race" : ""
    # `incremental` option
    build_type = ENV['incremental'] == "true" ? "-i" : "-a"
    # `tags` option
    build_tags = ENV['tags'] || "zstd snmp etcd zk docker ec2 gce cpython"

    # Check if we should use Embedded or System Python,
    # default to the embedded one.
    env = {}
    ldflags = []
    if !ENV["USE_SYSTEM_PY"]
      env["PKG_CONFIG_LIBDIR"] = "#{PKG_CONFIG_LIBDIR}"
      libdir = `PKG_CONFIG_LIBDIR="#{PKG_CONFIG_LIBDIR}" pkg-config --variable=libdir python-2.7`.strip
      fail "Can't find path to embedded lib directory with pkg-config" if libdir.empty?
      ldflags << "-r #{libdir}"
    end

    commit = `git rev-parse --short HEAD`.strip
    ldflags << "-X #{REPO_PATH}/pkg/version.commit=#{commit}"
    if ENV["WINDOWS_DELVE"]
      # On windows, need to build with the extra arguments -gcflags "-N -l" -ldflags="-linkmode internal"
      # if you want to be able to use the delve debugger.
      ldflags << "-linkmode internal"
      build_success = system(env, "go build #{race_opt} #{build_type} -tags '#{build_tags}' -o #{BIN_PATH}/#{agent_bin_name}  -gcflags \"-N -l\" -ldflags=\"#{ldflags.join(" ")}\" #{REPO_PATH}/cmd/agent")
    else
      build_success = system(env, "go build #{race_opt} #{build_type} -tags '#{build_tags}' -o #{BIN_PATH}/#{agent_bin_name} -ldflags \"#{ldflags.join(" ")}\" #{REPO_PATH}/cmd/agent")
    end
    fail "Agent build failed with code #{$?.exitstatus}" if !build_success

    Rake::Task["agent:refresh_assets"].invoke
  end

  desc "Refresh the build assets"
  task :refresh_assets do
    # Collector's assets and config files
    FileUtils.rm_rf("#{BIN_PATH}/dist")
    FileUtils.cp_r("./pkg/collector/dist/", "#{BIN_PATH}", :remove_destination => true)
    FileUtils.mv("#{BIN_PATH}/dist/agent", "#{BIN_PATH}/agent")
    FileUtils.chmod(0755, "#{BIN_PATH}/agent")
  end

  desc "Run the agent"
  task :run => %w[agent:build agent:run_lazy]

  desc "Run the agent (no build)"
  task :run_lazy do
    abort "Binary unavailable run agent:build or agent:run" if !File.exists? "#{BIN_PATH}/#{agent_bin_name}"
    sh("#{BIN_PATH}/agent start")
  end

  desc "Build omnibus installer"
  task :omnibus do
    # omnibus log level
    log_level = ENV["AGENT_OMNIBUS_LOG_LEVEL"] || "info"

    # omnibus config overrides
    overrides_cmd = ""
    overrides = []
    base_dir = ENV["AGENT_OMNIBUS_BASE_DIR"]
    if base_dir
      overrides.push("base_dir:#{base_dir}")
    end

    package_dir = ENV["AGENT_OMNIBUS_PACKAGE_DIR"]
    if package_dir
      overrides.push("package_dir:#{package_dir}")
    end

    Dir.chdir('omnibus') do
      system("bundle install --without development")

      if overrides.size > 0
        overrides_cmd = "--override=" + overrides.join(" ")
      end

    case os
      when "windows"
        system("omnibus.bat build datadog-agent6 --log-level=#{log_level} #{overrides_cmd}")
      else
        system("omnibus build datadog-agent6 --log-level=#{log_level} #{overrides_cmd}")
      end

    end
  end

  desc "Run agent system tests"
  task :system_test do
    system("cd #{ENV["GOPATH"]}/src/#{REPO_PATH}/test/integration/config_providers/zookeeper/ && bash ./test.sh") || exit(1)
  end

end
