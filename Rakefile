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

PKG_CONFIG_LIBDIR=File.join(`pwd`.strip, "pkg-config", os)
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
  TARGETS.each do |t|
    go_fmt(t)
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
    system("go vet #{t}/...")
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

      # check if we should use Embedded or System Python
      # default for testing is the System one, so we don't need to setup CI
      env = {}
      if ENV["USE_EMBEDDED_PY"]
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
    # check if we should use Embedded or System Python
    # default for build is the Embedded one
    env = {}
    if !ENV["USE_EMBEDDED_PY"]
      env["PKG_CONFIG_LIBDIR"] = "#{PKG_CONFIG_LIBDIR}"
    end

    system(env, "go build -o #{BIN_PATH}/agent.bin #{REPO_PATH}/cmd/agent")
    FileUtils.cp_r("./pkg/collector/check/py/dist/", "#{BIN_PATH}", :remove_destination => true)
    FileUtils.mv("#{BIN_PATH}/dist/agent", "#{BIN_PATH}/agent")
    FileUtils.chmod(0755, "#{BIN_PATH}/agent")
  end

  desc "Build omnibus installer"
  task :omnibus do
    Dir.chdir('omnibus') do
      system("bundle install --without development")
      # put omnibus stuff under ./var so that gitlab can cache it
      system("omnibus build datadog-agent6 --override=base_dir:var/")
    end
  end

end
