# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'datadog-agent'

dependency "python2" if with_python_runtime? "2"
dependency "python3" if with_python_runtime? "3"

dependency "libarchive" if windows?
dependency "yaml-cpp" if windows?

dependency "openscap" if linux? and !arm7l? and !heroku? # Security-agent dependency, not needed for Heroku

dependency 'datadog-agent-dependencies'

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

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
    major_version_arg = "%MAJOR_VERSION%"
    py_runtimes_arg = "%PY_RUNTIMES%"
    flavor_arg = "%AGENT_FLAVOR%"
  else
    env = {
        'GOPATH' => gopath.to_path,
        'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
        "Python2_ROOT_DIR" => "#{install_dir}/embedded",
        "Python3_ROOT_DIR" => "#{install_dir}/embedded",
        "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
        "CGO_CFLAGS" => "-I. -I#{install_dir}/embedded/include",
        "CGO_LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib"
    }
    major_version_arg = "$MAJOR_VERSION"
    py_runtimes_arg = "$PY_RUNTIMES"
    flavor_arg = "$AGENT_FLAVOR"
  end

  unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
    gomodcache = Pathname.new(ENV["OMNIBUS_GOMODCACHE"])
    env["GOMODCACHE"] = gomodcache.to_path
  end

  # include embedded path (mostly for `pkg-config` binary)
  env = with_embedded_path(env)

  # we assume the go deps are already installed before running omnibus
  if windows?
    platform = windows_arch_i386? ? "x86" : "x64"
    do_windows_sysprobe = ""
    if not windows_arch_i386? and ENV['WINDOWS_DDNPM_DRIVER'] and not ENV['WINDOWS_DDNPM_DRIVER'].empty?
      do_windows_sysprobe = "--windows-sysprobe"
    end
    command "inv -e rtloader.clean"
    command "inv -e rtloader.make --python-runtimes #{py_runtimes_arg} --install-prefix \"#{windows_safe_path(python_2_embedded)}\" --cmake-options \"-G \\\"Unix Makefiles\\\"\" --arch #{platform}", :env => env
    command "mv rtloader/bin/*.dll  #{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent/"
    command "inv -e agent.build --exclude-rtloader --python-runtimes #{py_runtimes_arg} --major-version #{major_version_arg} --rebuild --no-development --embedded-path=#{install_dir}/embedded --arch #{platform} #{do_windows_sysprobe} --flavor #{flavor_arg}", env: env
    command "inv -e systray.build --major-version #{major_version_arg} --rebuild --arch #{platform}", env: env
  else
    command "inv -e rtloader.clean"
    command "inv -e rtloader.make --python-runtimes #{py_runtimes_arg} --install-prefix \"#{install_dir}/embedded\" --cmake-options '-DCMAKE_CXX_FLAGS:=\"-D_GLIBCXX_USE_CXX11_ABI=0 -I#{install_dir}/embedded/include\" -DCMAKE_C_FLAGS:=\"-I#{install_dir}/embedded/include\" -DCMAKE_INSTALL_LIBDIR=lib -DCMAKE_FIND_FRAMEWORK:STRING=NEVER'", :env => env
    command "inv -e rtloader.install"
    command "inv -e agent.build --exclude-rtloader --python-runtimes #{py_runtimes_arg} --major-version #{major_version_arg} --rebuild --no-development --embedded-path=#{install_dir}/embedded --python-home-2=#{install_dir}/embedded --python-home-3=#{install_dir}/embedded --flavor #{flavor_arg}", env: env
  end

  if osx?
    conf_dir = "#{install_dir}/etc"
  else
    conf_dir = "#{install_dir}/etc/datadog-agent"
  end
  mkdir conf_dir
  mkdir "#{install_dir}/bin"
  unless windows?
    mkdir "#{install_dir}/run/"
    mkdir "#{install_dir}/scripts/"
  end

  ## build the custom action library required for the install
  if windows?
    platform = windows_arch_i386? ? "x86" : "x64"
    debug_customaction = ""
    if ENV['DEBUG_CUSTOMACTION'] and not ENV['DEBUG_CUSTOMACTION'].empty?
      debug_customaction = "--debug"
    end
    command "invoke -e customaction.build --major-version #{major_version_arg} #{debug_customaction} --arch=" + platform
  end

  # move around bin and config files
  move 'bin/agent/dist/datadog.yaml', "#{conf_dir}/datadog.yaml.example"
  if linux? or (windows? and not windows_arch_i386? and ENV['WINDOWS_DDNPM_DRIVER'] and not ENV['WINDOWS_DDNPM_DRIVER'].empty?)
      move 'bin/agent/dist/system-probe.yaml', "#{conf_dir}/system-probe.yaml.example"
  end
  move 'bin/agent/dist/conf.d', "#{conf_dir}/"

  unless windows?
    copy 'bin/agent', "#{install_dir}/bin/"
  else
    copy 'bin/agent/ddtray.exe', "#{install_dir}/bin/agent"
    copy 'bin/agent/dist', "#{install_dir}/bin/agent"
    mkdir Omnibus::Config.package_dir() unless Dir.exists?(Omnibus::Config.package_dir())
    copy 'bin/agent/customaction*.pdb', "#{Omnibus::Config.package_dir()}/"
  end

  block do
    # defer compilation step in a block to allow getting the project's build version, which is populated
    # only once the software that the project takes its version from (i.e. `datadog-agent`) has finished building
    platform = windows_arch_i386? ? "x86" : "x64"
    command "invoke trace-agent.build --python-runtimes #{py_runtimes_arg} --major-version #{major_version_arg} --arch #{platform} --flavor #{flavor_arg}", :env => env

    if windows?
      copy 'bin/trace-agent/trace-agent.exe', "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"
    else
      copy 'bin/trace-agent/trace-agent', "#{install_dir}/embedded/bin"
    end
  end

  if windows?
    platform = windows_arch_i386? ? "x86" : "x64"
    # Build the process-agent with the correct go version for windows
    command "invoke -e process-agent.build --python-runtimes #{py_runtimes_arg} --major-version #{major_version_arg} --arch #{platform} --flavor #{flavor_arg}", :env => env

    copy 'bin/process-agent/process-agent.exe', "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"

    unless windows_arch_i386?
      if ENV['WINDOWS_DDNPM_DRIVER'] and not ENV['WINDOWS_DDNPM_DRIVER'].empty?
        ## don't bother with system probe build on x86.
        command "invoke -e system-probe.build --windows"
        copy 'bin/system-probe/system-probe.exe', "#{Omnibus::Config.source_dir()}/datadog-agent/src/github.com/DataDog/datadog-agent/bin/agent"
      end
    end
  else
    command "invoke -e process-agent.build --python-runtimes #{py_runtimes_arg} --major-version #{major_version_arg} --flavor #{flavor_arg}", :env => env
    copy 'bin/process-agent/process-agent', "#{install_dir}/embedded/bin"
  end

  # Add SELinux policy for system-probe
  if debian? || redhat?
    mkdir "#{conf_dir}/selinux"
    command "inv -e selinux.compile-system-probe-policy-file --output-directory #{conf_dir}/selinux", env: env
  end

  # Security agent
  unless windows? || heroku?
    command "invoke -e security-agent.build --major-version #{major_version_arg}", :env => env
    copy 'bin/security-agent/security-agent', "#{install_dir}/embedded/bin"
    move 'bin/agent/dist/security-agent.yaml', "#{conf_dir}/security-agent.yaml.example"
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
      erb source: "upstart_debian.security.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-security.conf",
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
      erb source: "sysvinit_debian.trace.erb",
          dest: "#{install_dir}/scripts/datadog-agent-trace",
          mode: 0755,
          vars: { install_dir: install_dir, etc_dir: etc_dir }
      erb source: "sysvinit_debian.security.erb",
          dest: "#{install_dir}/scripts/datadog-agent-security",
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
      erb source: "upstart_redhat.security.conf.erb",
          dest: "#{install_dir}/scripts/datadog-agent-security.conf",
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
    erb source: "systemd.security.service.erb",
        dest: "#{install_dir}/scripts/datadog-agent-security.service",
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
    # Add @executable_path/../Frameworks to rpath to find the swift libs in the Frameworks folder.
    command 'swiftc -O -swift-version "5" -target "x86_64-apple-macosx10.10" -Xlinker \'-rpath\' -Xlinker \'@executable_path/../Frameworks\' Sources/*.swift -o gui', cwd: systray_build_dir
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
