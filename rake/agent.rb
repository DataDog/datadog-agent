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

  desc "Build the agent, pass 'race=true' to invoke the race detector"
  task :build do
    # -race option
    race_opt = ENV['race'] == "true" ? "-race" : ""

    # Check if we should use Embedded or System Python,
    # default to the embedded one.
    env = {}
    if !ENV["USE_SYSTEM_PY"]
      env["PKG_CONFIG_LIBDIR"] = "#{PKG_CONFIG_LIBDIR}"
    end

    commit = `git rev-parse --short HEAD`.strip
    ldflags = "-X #{REPO_PATH}/pkg/version.commit=#{commit}"
    system(env, "go build #{race_opt} -o #{BIN_PATH}/#{agent_bin_name} -ldflags \"#{ldflags}\" #{REPO_PATH}/cmd/agent")
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
  task :run => %w[agent:build] do
    sh("#{BIN_PATH}/agent start -f")
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

      system("omnibus.bat build datadog-agent6 --log-level=#{log_level} #{overrides_cmd}")
    end
  end

end
