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

def pkg_config_libdir
  path = ENV["CONDA_PREFIX"]
  if path.to_s == ""
    fail "No active conda enviroment detected. You can create one running 'rake py'."
  end

  return path + "/lib/pkgconfig"
end

ORG_PATH="github.com/DataDog"
REPO_PATH="#{ORG_PATH}/datadog-agent"
TARGETS = %w[./pkg ./cmd]

CLOBBER.include("*.cov")

task default: %w[agent:build]

desc "Setup the UPE with Conda"
task :py do
  system("curl -sSO https://gist.githubusercontent.com/masci/6351be014f6950e0f9918c3337034e40/raw/537a53f0ac072c180eca8e475fb6f3ef5bd735e5/datadog-agent.yaml")
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

      env = {"PKG_CONFIG_LIBDIR" => "#{pkg_config_libdir}"}
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
    env = {"PKG_CONFIG_LIBDIR" => "#{pkg_config_libdir}"}
    system(env, "go build -o #{BIN_PATH}/#{exe_name} #{REPO_PATH}/cmd/agent")
    Rake::Task["agent:refresh_assets"].invoke
  end

  desc "Refresh build assets for the Agent"
  task :refresh_assets do
    # Create target dist folder from scratch
    FileUtils.rm_rf("#{BIN_PATH}/dist")
    FileUtils.cp_r("./pkg/collector/dist/", "#{BIN_PATH}", :remove_destination => true)

    # Unless it's omnibus calling, embed the Python environment from conda
    if ENV["copyenv"] != "false"
      env_path = ''
      output = `conda env list`
      output.split("\n").each do |line|      
        toks = line.split()
        if toks[0] == "datadog-agent"
          env_path = toks[1]
          break
        end
      end
      FileUtils.cp_r("#{env_path}", "#{BIN_PATH}/dist/python")
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

      system("omnibus build agent --log-level=#{log_level} #{overrides_cmd}")
    end
  end

end
