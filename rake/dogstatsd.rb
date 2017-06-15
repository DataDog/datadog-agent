require_relative './common'


namespace :dogstatsd do
  DOGSTATSD_BIN_PATH="./bin/dogstatsd"
  STATIC_BIN_PATH="./bin/static"
  CLOBBER.include(DOGSTATSD_BIN_PATH, STATIC_BIN_PATH)

  desc "Build Dogstatsd [race=false|incremental=false|static=false]"
  task :build do
    race_opt = ENV['race'] == "true" ? "-race" : ""
    build_type_opt = ENV['incremental'] == "true" ? "-i" : "-a"
    static_bin = ENV['static'] == "true"

    bin_path = DOGSTATSD_BIN_PATH
    commit = `git rev-parse --short HEAD`.strip
    ldflags = "-X #{REPO_PATH}/pkg/version.commit=#{commit}"
    if static_bin then
      ldflags += "-s -w -extldflags \"-static\""
      bin_path = STATIC_BIN_PATH
    end

    system("go build #{race_opt} #{build_type_opt} -o #{bin_path}/#{bin_name("dogstatsd")} -ldflags \"#{ldflags}\" #{REPO_PATH}/cmd/dogstatsd/")
  end

  desc "Run Dogstatsd"
  task :run => %w[dogstatsd:build] do
    system("#{DOGSTATSD_BIN_PATH}/dogstatsd")
  end

  desc "Run Dogstatsd system tests [skip_rebuild=false]"
  task :system_test do
    if ENV['skip_rebuild'] != "true" then
      puts "Building DogStatsD"
      Rake::Task["dogstatsd:build"].invoke
    end

    puts "Starting DogStatsD system tests"
    root = `git rev-parse --show-toplevel`.strip
    bin_path = File.join(root, DOGSTATSD_BIN_PATH, "dogstatsd")
    system("DOGSTATSD_BIN=\"#{bin_path}\" go test -v #{REPO_PATH}/test/system/dogstatsd/") || exit(1)
  end

  desc "Run Dogstatsd size test [skip_rebuild=false]"
  task :size_test do
    if ENV['skip_rebuild'] != "true" then
      puts "Building DogStatsD"
      ENV['static'] = "true"
      Rake::Task["dogstatsd:build"].invoke
    end

    root = `git rev-parse --show-toplevel`.strip
    bin_path = File.join(root, STATIC_BIN_PATH, "dogstatsd")
    size = File.size(bin_path) / 1024

    if size > 15 * 1024 then
      puts "DogStatsD static build size too big: #{size} kB"
      puts "This means your PR added big classes or dependencies in the packages dogstatsd uses"
      exit(1)
    else
      puts "DogStatsD static build size OK: #{size} kB"
    end
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
