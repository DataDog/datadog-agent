require_relative './common'


namespace :dogstatsd do
  DOGSTATSD_BIN_PATH="./bin/dogstatsd"
  CLOBBER.include(DOGSTATSD_BIN_PATH)

  STATIC_BIN_PATH="./bin/static"
  STATIC_GO_FLAGS="--ldflags '-s -w -extldflags \"-static\"'"

  desc "Build Dogstatsd"
  task :build do
    # -race option
    race_opt = ENV['race'] == "true" ? "-race" : ""
    build_type = ENV['incremental'] == "true" ? "-i" : "-a"

    commit = `git rev-parse --short HEAD`.strip
    ldflags = "-X #{REPO_PATH}/pkg/version.commit=#{commit}"

    system("go build #{race_opt} #{build_type} -o #{DOGSTATSD_BIN_PATH}/#{bin_name("dogstatsd")} -ldflags \"#{ldflags}\" #{REPO_PATH}/cmd/dogstatsd/")
  end

  desc "Build static Dogstatsd"
  task :build_static do
    system("go build #{STATIC_GO_FLAGS} -o #{STATIC_BIN_PATH}/#{bin_name("dogstatsd")} #{REPO_PATH}/cmd/dogstatsd/")
  end

  desc "Run Dogstatsd"
  task :run => %w[dogstatsd:build] do
    system("#{DOGSTATSD_BIN_PATH}/dogstatsd")
  end

  desc "Run Dogstatsd system tests"
  task :system_test do
    if ENV['skip_rebuild'] == "true" then
      puts "Skipping DogStatsD build"
    else
      puts "Building DogStatsD"
      Rake::Task["dogstatsd:build"].invoke
    end

    puts "Starting DogStatsD system tests"
    root = `git rev-parse --show-toplevel`.strip
    bin_path = File.join(root, DOGSTATSD_BIN_PATH, "dogstatsd")
    system("DOGSTATSD_BIN=\"#{bin_path}\" go test -v #{REPO_PATH}/test/system/dogstatsd/") || exit(1)
  end

  desc "Run Dogstatsd size test"
  task :size_test do
    if ENV['skip_rebuild'] == "true" then
      puts "Skipping DogStatsD build"
    else
      puts "Building DogStatsD"
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
