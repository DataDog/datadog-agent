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

def exe_name
  case os
  when "windows"
    "agent.exe"
  else
    "agent.bin"
  end
end

def condapath
  path = ENV["CONDA_PREFIX"]
  if path.to_s == ""
    fail "No active conda enviroment detected. You can create one running 'rake py'."
  end

  path
end

def libdir
  return condapath + "/lib"
end

def pkg_config_libdir
  case os
  when "windows"
    "./cmd/agent/build/win"
  else
    return libdir + "/pkgconfig"
  end
end

def env_definition_path
  case os
  when "windows"
    "https://gist.githubusercontent.com/masci/d6413e9f501d7fecad52b8fdbf80117b/raw/ce8cea77dc1b3a1399ac3ba5d2b2c87ed7904dab/datadog-agent.yaml"
  else
    "https://gist.githubusercontent.com/masci/6351be014f6950e0f9918c3337034e40/raw/537a53f0ac072c180eca8e475fb6f3ef5bd735e5/datadog-agent.yaml"
  end
end

def build_env
  env = {"PKG_CONFIG_LIBDIR" => "#{pkg_config_libdir}"}

  case os
  when "linux"
    env["LD_LIBRARY_PATH"] = condapath + '/lib'
  when "darwin"
    # not sure if we need to instrument the dynamic linker on OSX
    # let's use the fallback env var so we don't interfere with the
    # system
    env["DYLD_FALLBACK_LIBRARY_PATH"] = libdir
  end

  env
end

ORG_PATH="github.com/DataDog"
REPO_PATH="#{ORG_PATH}/datadog-agent"
TARGETS = %w[./pkg ./cmd]

CLOBBER.include("*.cov")

task default: %w[agent:build]

desc "Setup the UPE with Conda"
task :py do
  res = system("curl -sSO #{env_definition_path}")
  puts "curl -sSO #{env_definition_path}"
  if res != true
    fail "Unable to download definition file at #{env_definition_path}"
  end
  system("conda env create --force -q -f datadog-agent.yaml")
end

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

desc "Run testsuite"
task :test => %w[fmt lint vet] do
  PROFILE = "profile.cov"  # collect global coverage data in this file
  `echo "mode: count" > #{PROFILE}`

  TARGETS.each do |t|
    Dir.glob("#{t}/**/*").select {|f| File.directory? f }.each do |pkg_folder|  # recursively search for go packages
      next if Dir.glob(File.join(pkg_folder, "*.go")).length == 0  # folder is a package if contains go modules
      profile_tmp = "#{pkg_folder}/profile.tmp"  # temp file to collect coverage data

      system(build_env, "go test -short -covermode=count -coverprofile=#{profile_tmp} #{pkg_folder}") || exit(1)
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
    system(build_env, "go build -o #{BIN_PATH}/#{exe_name} #{REPO_PATH}/cmd/agent")
    Rake::Task["agent:refresh_assets"].invoke
  end

  desc "Refresh build assets for the Agent"
  task :refresh_assets do
    # Create target dist folder from scratch
    FileUtils.rm_rf("#{BIN_PATH}/dist")
    FileUtils.cp_r("./pkg/collector/dist/", "#{BIN_PATH}", :remove_destination => true)

    # Unless it's omnibus calling, embed the Python environment from conda
    if ENV["copyenv"] != "false"
      FileUtils.cp_r(condapath, "#{BIN_PATH}/dist/python")
    end

    # setup the entrypoint script at the root bin folder
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

    # clean up any partial build
    system("rm -rf ./bin/*")

    Dir.chdir('omnibus') do
      system("bundle install --without development")

      if overrides.size > 0
        overrides_cmd = "--override=" + overrides.join(" ")
      end

      system(build_env, "omnibus build agent --log-level=#{log_level} #{overrides_cmd}")
    end
  end

end
