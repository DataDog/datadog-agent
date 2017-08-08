require_relative './rake/common'
require_relative './rake/agent'
require_relative './rake/dogstatsd'
require_relative './rake/benchmarks'
require_relative './rake/docker'
require_relative './rake/py-launcher'

# Only add 'embedded' pkg-config dir to the env's PKG_CONFIG_PATH if the embedded python/libs are used
# In that case it should be the first path so that the 'embedded' dir takes precedence over the 'system' one
PKG_CONFIG_EMBEDDED_PATH=File.join(Dir.pwd, "pkg-config", os, "embedded")
# Always add 'system' pkg-config dir to the env's PKG_CONFIG_PATH
PKG_CONFIG_PATH=File.join(Dir.pwd, "pkg-config", os, "system")
ENV["PKG_CONFIG_PATH"] = ENV.has_key?("PKG_CONFIG_PATH") ? "#{PKG_CONFIG_PATH}:#{ENV['PKG_CONFIG_PATH']}" : PKG_CONFIG_PATH

ORG_PATH="github.com/DataDog"
REPO_PATH="#{ORG_PATH}/datadog-agent"
TARGETS = %w[./pkg ./cmd]

CLOBBER.include("*.cov")

task default: %w[agent:build]

desc "Setup Go dependencies"
task :deps do
  system("go get -u github.com/golang/dep/cmd/dep")
  system("go get -u github.com/golang/lint/golint")
  system("dep ensure")
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
      if !ENV["USE_SYSTEM_LIBS"]
        env["PKG_CONFIG_PATH"] = "#{PKG_CONFIG_EMBEDDED_PATH}:#{ENV["PKG_CONFIG_PATH"]}"
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
task system_test: %w[dogstatsd:system_test] # FIXME: re-enable pylauncher:system_test once they're fixed

desc "Build allthethings"
task build: %w[agent:build dogstatsd:build pylauncher:build]
