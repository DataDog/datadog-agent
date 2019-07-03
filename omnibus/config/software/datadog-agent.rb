# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'datadog-agent'

dependency "python2" if with_python_runtime? "2"
dependency "python3" if with_python_runtime? "3"

license "Apache-2.0"
license_file "../LICENSE"

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  etc_dir = "/etc/datadog-agent"
  if windows?
    env = {
        'GOPATH' => gopath.to_path,
        'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
        "Python2_ROOT_DIR" => "#{windows_safe_path(python_2_embedded)}",
        "Python3_ROOT_DIR" => "#{windows_safe_path(python_3_embedded)}",
        "CMAKE_INSTALL_PREFIX" => "#{windows_safe_path(python_2_embedded)}",
    }
  else
    env = {
        'GOPATH' => gopath.to_path,
        'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
        "Python2_ROOT_DIR" => "#{install_dir}/embedded",
        "Python3_ROOT_DIR" => "#{install_dir}/embedded",
        "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
    }
  end

  # include embedded path (mostly for `pkg-config` binary)
  env = with_embedded_path(env)

  # we assume the go deps are already installed before running omnibus
  if windows?
    command "inv -e rtloader.build --install-prefix \"#{windows_safe_path(python_2_embedded)}\" --cmake-options \"-G \\\"Unix Makefiles\\\"\"", :env => env
    command "mv rtloader/bin/*.dll  #{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent/"
    command "inv -e agent.build --rtloader-root=#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/rtloader --rebuild --no-development --embedded-path=#{install_dir}/embedded", env: env
    command "inv -e systray.build --rebuild --no-development", env: env
  else
    command "inv -e rtloader.build --install-prefix \"#{install_dir}/embedded\" --cmake-options '-DCMAKE_CXX_FLAGS:=\"-D_GLIBCXX_USE_CXX11_ABI=0\" -DCMAKE_INSTALL_LIBDIR=lib -DCMAKE_FIND_FRAMEWORK:STRING=NEVER'", :env => env
    command "inv -e rtloader.install"
    command "inv -e agent.build --rebuild --no-development --embedded-path=#{install_dir}/embedded --python-home-2=#{install_dir}/embedded --python-home-3=#{install_dir}/embedded", env: env
  end

  if osx?
    conf_dir = "#{install_dir}/etc"
  else
    conf_dir = "#{install_dir}/etc/datadog-agent"
  end
  mkdir conf_dir
  unless windows?
    mkdir "#{install_dir}/run/"
    mkdir "#{install_dir}/scripts/"
  end

  ## build the custom action library required for the install
  if windows?
    command "invoke customaction.build"
  end

  # move around bin and config files
  move 'bin/agent/dist/datadog.yaml', "#{conf_dir}/datadog.yaml.example"
  move 'bin/agent/dist/system-probe.yaml', "#{conf_dir}/system-probe.yaml.example"
  move 'bin/agent/dist/conf.d', "#{conf_dir}/"

  copy 'bin', install_dir


  block do
    # defer compilation step in a block to allow getting the project's build version, which is populated
    # only once the software that the project takes its version from (i.e. `datadog-agent`) has finished building
    env['TRACE_AGENT_VERSION'] = project.build_version.gsub(/[^0-9\.]/, '') # used by gorake.rb in the trace-agent, only keep digits and dots
    command "invoke trace-agent.build", :env => env

    if windows?
      copy 'bin/trace-agent/trace-agent.exe', "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"
    else
      copy 'bin/trace-agent/trace-agent', "#{install_dir}/embedded/bin"
    end
  end


  if windows?
    # Build the process-agent with the correct go version for windows
    command "invoke -e process-agent.build", :env => env

    copy 'bin/process-agent/process-agent.exe', "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"
  else
    # TODO(processes): change this to be ebpf:latest when we move to go1.12.x on the agent
    command "invoke -e process-agent.build --go-version=1.10.1", :env => env
    copy 'bin/process-agent/process-agent', "#{install_dir}/embedded/bin"
  end


  # Build the system-probe
  if linux?
    command "invoke -e system-probe.build --go-version=1.10.1", :env => env
    copy 'bin/system-probe/system-probe', "#{install_dir}/embedded/bin"
    block { File.chmod(0755, "#{install_dir}/embedded/bin/system-probe") }
  end


  if linux?
    if debian?
      erb source: "upstart_debian.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.process.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-process.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.sysprobe.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-sysprobe.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_debian.trace.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-trace.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.erb",
          dest: "#{install_dir}/scripts/datadog-agent",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.process.erb",
          dest: "#{install_dir}/scripts/datadog-agent-process",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.sysprobe.erb",
          dest: "#{install_dir}/scripts/datadog-agent-sysprobe",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.trace.erb",
          dest: "#{install_dir}/scripts/datadog-agent-trace",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
    elsif redhat? || suse?
      # Ship a different upstart job definition on RHEL to accommodate the old
      # version of upstart (0.6.5) that RHEL 6 provides.
      erb source: "upstart_redhat.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.process.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-process.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.sysprobe.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-sysprobe.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "upstart_redhat.trace.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-trace.conf",
          mode: 0644,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
    end

    erb source: "systemd.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent.service",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    erb source: "systemd.process.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent-process.service",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    erb source: "systemd.sysprobe.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent-sysprobe.service",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
    erb source: "systemd.trace.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent-trace.service",
        mode: 0644,
        vars: { install_dir: install_dir, etc_dir: etc_dir }
  end

  if osx?
    # Launchd service definition
    erb source: "launchd.plist.example.erb",
        dest: "#{conf_dir}/com.datadoghq.agent.plist.example",
        mode: 0644,
        vars: { install_dir: install_dir }

    # Systray GUI
    app_temp_dir = "#{install_dir}/Datadog Agent.app/Contents"
    mkdir "#{app_temp_dir}/MacOS"
    systray_build_dir = "#{project_dir}/cmd/agent/gui/systray"
    # Target OSX 10.10 (it brings significant changes to Cocoa and Foundation APIs, and older versions of OSX are EOL'ed)
    command 'swiftc -O -swift-version "3" -target "x86_64-apple-macosx10.10" -static-stdlib Sources/*.swift -o gui', cwd: systray_build_dir
    copy "#{systray_build_dir}/gui", "#{app_temp_dir}/MacOS/"
    copy "#{systray_build_dir}/agent.png", "#{app_temp_dir}/MacOS/"
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  unless windows?
    delete "#{install_dir}/uselessfile"
  end
end
