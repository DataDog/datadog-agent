require 'rake/clean'
require './go'

def os
  case RUBY_PLATFORM
  when /linux/
    "linux"
  when /darwin/
    "darwin"
  else
    fail 'Unsupported OS'
  end
end

PROJECT_DIR=`pwd`.strip
PKG_CONFIG_LIBDIR=File.join(PROJECT_DIR, "pkg-config", os)
ORG_PATH="github.com/DataDog"
REPO_PATH="#{ORG_PATH}/datadog-agent"
TARGETS = %w[./pkg ./cmd]

CLOBBER.include("*.cov")

task default: %w[agent:build]

desc "Setup Go dependencies"
task :deps do
  system("go get github.com/Masterminds/glide")
  system("go get -u github.com/golang/lint/golint")
  system("glide up")
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

desc "Run testsuite"
task :test => %w[fmt lint vet] do
  PROFILE = "profile.cov"  # collect global coverage data in this file
  `echo "mode: count" > #{PROFILE}`

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

      system(env, "go test -short -covermode=count -coverprofile=#{profile_tmp} #{pkg_folder}") || exit(1)
      if File.file?(profile_tmp)
        `cat #{profile_tmp} | tail -n +2 >> #{PROFILE}`
        File.delete(profile_tmp)
      end
    end
  end

  sh("go tool cover -func #{PROFILE}")
end

desc "Build allthethings"
task build: %w[agent:build]

namespace :agent do
  BIN_PATH="./bin/agent"
  CLOBBER.include(BIN_PATH)

  desc "Build the agent"
  task :build do
    # Check if we should use Embedded or System Python,
    # default to the embedded one.
    env = {}
    if !ENV["USE_SYSTEM_PY"]
      env["PKG_CONFIG_LIBDIR"] = "#{PKG_CONFIG_LIBDIR}"
    end

    system(env, "go build -o #{BIN_PATH}/agent.bin #{REPO_PATH}/cmd/agent")
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
