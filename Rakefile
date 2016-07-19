require 'rake/clean'
require './go'

PKG_CONFIG_LIBDIR=File.join(`pwd`.strip, "pkg-config", RUBY_PLATFORM)
ORG_PATH="github.com/DataDog"
REPO_PATH="#{ORG_PATH}/datadog-agent"
TARGETS = %w[./pkg ./cmd]

CLOBBER.include("*.cov")

task default: %w[agent:build]

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
      system("go test -short -covermode=count -coverprofile=#{profile_tmp} #{pkg_folder}")
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
    system({"PKG_CONFIG_LIBDIR" => "#{PKG_CONFIG_LIBDIR}"}, "go build -o #{BIN_PATH}/agent #{REPO_PATH}/cmd/agent")
    FileUtils.cp_r("./pkg/collector/check/py/dist/", "#{BIN_PATH}/dist/")
  end

end
