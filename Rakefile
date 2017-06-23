require_relative './rake/common'
require_relative './rake/agent'
require_relative './rake/dogstatsd'
require_relative './rake/benchmarks'
require_relative './rake/docker'

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

desc "Run testsuite [race=false|tags=*|puppy=false]"
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

      system(env, "go test -tags '#{go_build_tags}' #{race_opt} -short #{covermode_opt} -coverprofile=#{profile_tmp} #{pkg_folder}") || exit(1)
      if File.file?(profile_tmp)
        `cat #{profile_tmp} | tail -n +2 >> #{PROFILE}`
        File.delete(profile_tmp)
      end
    end
  end

  sh("go tool cover -func #{PROFILE}")
end

desc "Run every system tests"
task system_test: %w[dogstatsd:system_test]

desc "Build allthethings"
task build: %w[agent:build dogstatsd:build]
