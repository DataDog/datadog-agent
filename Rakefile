require 'rake/clean'
require './go'

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

def agent_bin_name
  case os
  when "windows"
    "agent.exe"
  else
    "agent.bin"
  end
end

def dogstatsd_bin_name
  case os
  when "windows"
    "dogstatsd.exe"
  else
    "dogstatsd"
  end
end

PKG_CONFIG_LIBDIR=File.join(Dir.pwd, "pkg-config", os)
ORG_PATH="github.com/DataDog"
REPO_PATH="#{ORG_PATH}/datadog-agent"
TARGETS = %w[./pkg ./cmd]

CLOBBER.include("*.cov")

task default: %w[agent:build]

desc "Setup Go dependencies"
task :deps do
  system("go get github.com/Masterminds/glide")
  system("go get -u github.com/golang/lint/golint")
  system("glide install")
end

desc "Run go fmt on #{TARGETS}"
task :fmt do
  fail_on_mod = ENV["CI"] # only fail on modification when we're running in CI env
  TARGETS.each do |t|
    go_fmt(t, fail_on_mod)
  end
end

desc "Run golint on #{TARGETS}"
task :lint do
  TARGETS.each do |t|
    go_lint(t)
  end
end

desc "Run go vet on #{TARGETS}"
task :vet do
  TARGETS.each do |t|
    go_vet(t)
  end
end

desc "Run testsuite, pass 'race=true' to invoke the race detector"
task :test => %w[fmt lint vet] do
  PROFILE = "profile.cov"  # collect global coverage data in this file
  `echo "mode: count" > #{PROFILE}`
  covermode_opt = "-covermode=count"

  # -race option
  race_opt = ENV['race'] == "true" ? "-race" : ""
  if race_opt != ""
    # atomic is quite expensive but it's the only way to run
    # both the coverage and the race detector at the same time
    # without getting false positives from the cover counter
    covermode_opt = "-covermode=atomic"
  end

  TARGETS.each do |t|
    Dir.glob("#{t}/**/*").select {|f| File.directory? f }.each do |pkg_folder|  # recursively search for go packages
      next if Dir.glob(File.join(pkg_folder, "*.go")).length == 0  # folder is a package if contains go modules
      profile_tmp = "#{pkg_folder}/profile.tmp"  # temp file to collect coverage data

      # Check if we should use Embedded or System Python,
      # default to the embedded one.
      env = {}
      if !ENV["USE_SYSTEM_PY"]
        env["PKG_CONFIG_LIBDIR"] = "#{PKG_CONFIG_LIBDIR}"
      end

      system(env, "go test #{race_opt} -short #{covermode_opt} -coverprofile=#{profile_tmp} #{pkg_folder}") || exit(1)
      if File.file?(profile_tmp)
        `cat #{profile_tmp} | tail -n +2 >> #{PROFILE}`
        File.delete(profile_tmp)
      end
    end
  end

  sh("go tool cover -func #{PROFILE}")
end

desc "Build allthethings"
task build: %w[agent:build dogstatsd:build]

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

    system(env, "go build #{race_opt} -o #{BIN_PATH}/#{exe_name} #{REPO_PATH}/cmd/agent")
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

      system("omnibus build datadog-agent6 --log-level=#{log_level} #{overrides_cmd}")
    end
  end

end
