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
  desc "Build the Agent [race=false|incremental=false|tags=*|puppy=false]"
  task :build do
    # `race` option
    race_opt = ENV['race'] == "true" ? "-race" : ""
    # `incremental` option
    build_type = ENV['incremental'] == "true" ? "-i" : "-a"

    # Check if we should use Embedded or System Python,
    # default to the embedded one.
    env = {}
    gcflags = []
    ldflags = get_base_ldflags()
    if !ENV["USE_SYSTEM_LIBS"]
      env["PKG_CONFIG_PATH"] = "#{PKG_CONFIG_EMBEDDED_PATH}" + File::PATH_SEPARATOR + "#{ENV["PKG_CONFIG_PATH"]}"
      ENV["PKG_CONFIG_PATH"] = "#{PKG_CONFIG_EMBEDDED_PATH}" + File::PATH_SEPARATOR + "#{ENV["PKG_CONFIG_PATH"]}"
      libdir = `pkg-config --variable=libdir python-2.7`.strip
      fail "Can't find path to embedded lib directory with pkg-config" if libdir.empty?
      ldflags << "-r #{libdir}"
    else
      if os == "windows"
        # set PKG_CONFIG_SYSTEM to point to where your local .pc files are. ENV["PKG_CONFIG_PATH"] is already set up,
        # and you can't really concatenate onto it.
        env["PKG_CONFIG_PATH"] = "#{ENV["PKG_CONFIG_SYSTEM"]}" + File::PATH_SEPARATOR + "#{ENV["PKG_CONFIG_PATH"]}"
        ENV["PKG_CONFIG_PATH"] = "#{ENV["PKG_CONFIG_SYSTEM"]}" + File::PATH_SEPARATOR + "#{ENV["PKG_CONFIG_PATH"]}"
        libdir = `pkg-config --variable=libdir python-2.7`.strip
        fail "Can't find path to embedded lib directory with pkg-config" if libdir.empty?
        ldflags << "-r #{libdir}"
      end
    end

    if os == "windows"
      # This generates the manifest resource. The manifest resource is necessary for
      # being able to load the ancient C-runtime that comes along with Python 2.7

      # fixme -- still need to calculate correct *_VER numbers at build time rather than
      # hard-coded here.
      command = "windres --define MAJ_VER=6 --define MIN_VER=0 --define PATCH_VER=0 -i cmd/agent/agent.rc --target=pe-x86-64 -O coff -o cmd/agent/rsrc.syso"
      puts command
      build_success = system(env, command)
      fail "Agent build failed with code #{$?.exitstatus}" if !build_success
    end
    if ENV["DELVE"]
      gcflags << "-N" << "-l"
      if os == "windows"
        # On windows, need to build with the extra argument -ldflags="-linkmode internal"
        # if you want to be able to use the delve debugger.
        ldflags << "-linkmode internal"
      end
    end

    command = "go build #{race_opt} #{build_type} -tags \"#{go_build_tags}\" -o #{BIN_PATH}/#{agent_bin_name} -gcflags=\"#{gcflags.join(" ")}\" -ldflags=\"#{ldflags.join(" ")}\" #{REPO_PATH}/cmd/agent"
    puts command
    build_success = system(env, command)
    fail "Agent build failed with code #{$?.exitstatus}" if !build_success

    Rake::Task["agent:refresh_assets"].invoke
  end

  desc "Refresh the build assets"
  task :refresh_assets do
    # Collector's assets and config files
    FileUtils.rm_rf("#{BIN_PATH}/dist")
    FileUtils.cp_r("./pkg/collector/dist/", "#{BIN_PATH}", :remove_destination => true)
    FileUtils.cp_r("./pkg/status/dist/", "#{BIN_PATH}", :remove_destination => true)
    FileUtils.cp_r("./dev/dist/", "#{BIN_PATH}", :remove_destination => true)
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

  desc "Build omnibus installer [puppy=false]"
  task :omnibus do
    # omnibus config overrides
    overrides = []

    # puppy mode option
    project_name = ENV['puppy'] == "true" ? "puppy" : "datadog-agent6"

    # omnibus log level
    log_level = ENV["AGENT_OMNIBUS_LOG_LEVEL"] || "info"

    base_dir = ENV["AGENT_OMNIBUS_BASE_DIR"]
    if base_dir
      overrides.push("base_dir:#{base_dir}")
    end

    package_dir = ENV["AGENT_OMNIBUS_PACKAGE_DIR"]
    if package_dir
      overrides.push("package_dir:#{package_dir}")
    end

    overrides_cmd = ""
    if overrides.size > 0
      overrides_cmd = "--override=" + overrides.join(" ")
    end

    Dir.chdir('omnibus') do
      system("bundle install --without development")
      case os
      when "windows"
        system("bundle exec omnibus.bat build #{project_name} --log-level=#{log_level} #{overrides_cmd}")
      else
        system("omnibus build #{project_name} --log-level=#{log_level} #{overrides_cmd}")
      end
    end

  end

  desc "Run agent system tests"
  task :system_test do
    system("cd #{ENV["GOPATH"]}/src/#{REPO_PATH}/test/integration/config_providers/zookeeper/ && bash ./test.sh") || exit(1)
  end

end
