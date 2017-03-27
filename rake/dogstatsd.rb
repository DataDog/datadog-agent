require_relative './common'


def dogstatsd_bin_name
  case os
  when "windows"
    "dogstatsd.exe"
  else
    "dogstatsd"
  end
end

namespace :dogstatsd do
  DOGSTATSD_BIN_PATH="./bin/dogstatsd"
  CLOBBER.include(DOGSTATSD_BIN_PATH)

  desc "Build Dogstatsd"
  task :build do
    system("go build -o #{DOGSTATSD_BIN_PATH}/#{dogstatsd_bin_name} #{REPO_PATH}/cmd/dogstatsd/")
  end

  desc "Run Dogstatsd"
  task :run => %w[dogstatsd:build] do
    system("#{DOGSTATSD_BIN_PATH}/dogstatsd")
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

      system("omnibus build dogstatsd --log-level=#{log_level} #{overrides_cmd}")
    end
  end
end
