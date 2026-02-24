# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require './lib/fips.rb'
require './lib/project_helpers.rb'
require 'pathname'

name 'datadog-agent'

# We don't want to build any dependencies in "repackaging mode" so all usual dependencies
# need to go under this guard.
unless do_repackage?
  # creates required build directories
  dependency 'datadog-agent-prepare'

  dependency "python3"

  dependency "openscap" if linux_target? and !arm7l_target? and !heroku_target? # Security-agent dependency, not needed for Heroku

  dependency 'datadog-agent-dependencies'
end

source path: '..',
       options: {
         exclude: ["**/.cache/**/*", "**/testdata/**/*"],
       }
relative_path 'src/github.com/DataDog/datadog-agent'

always_build true

build do
  license :project_license

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  flavor_arg = ENV['AGENT_FLAVOR']
  fips_args = fips_mode? ? "--fips-mode" : ""
  # include embedded path (mostly for `pkg-config` binary)
  #
  # with_embedded_path prepends the embedded path to the PATH from the global environment
  # in particular it ignores the PATH from the environment given as argument
  # so we need to call it before setting the PATH
  env = with_embedded_path()
  env = {
    'GOPATH' => gopath.to_path,
    'PATH' => ["#{gopath.to_path}/bin", env['PATH']].join(File::PATH_SEPARATOR),
  }
  unless windows_target?
    env['LDFLAGS'] = "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib"
    env['CGO_CFLAGS'] = "-I. -I#{install_dir}/embedded/include"
    env['CGO_LDFLAGS'] = "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib"
  end

  unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
    gomodcache = Pathname.new(ENV["OMNIBUS_GOMODCACHE"])
    env["GOMODCACHE"] = gomodcache.to_path
  end

  env = with_standard_compiler_flags(env)
  if fips_mode?
    add_msgo_to_env(env)
  end

  # we assume the go deps are already installed before running omnibus
  if windows_target?
    platform = windows_arch_i386? ? "x86" : "x64"
    do_windows_sysprobe = ""
    if not windows_arch_i386? and ENV['WINDOWS_DDNPM_DRIVER'] and not ENV['WINDOWS_DDNPM_DRIVER'].empty?
      do_windows_sysprobe = "--windows-sysprobe"
    end
    command "dda inv -- -e rtloader.clean", :live_stream => Omnibus.logger.live_stream(:info)
    command "dda inv -- -e rtloader.make --install-prefix \"#{windows_safe_path(python_3_embedded)}\" --cmake-options \"-G \\\"Unix Makefiles\\\" \\\"-DPython3_EXECUTABLE=#{windows_safe_path(python_3_embedded)}\\python.exe\\\" \\\"-DCMAKE_BUILD_TYPE=RelWithDebInfo\\\"\"", :env => env, :live_stream => Omnibus.logger.live_stream(:info)
    command "mv rtloader/bin/*.dll  #{install_dir}/bin/agent/"
    command "dda inv -- -e agent.build --exclude-rtloader --no-development --install-path=#{install_dir} --embedded-path=#{install_dir}/embedded #{do_windows_sysprobe} --flavor #{flavor_arg}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    command "dda inv -- -e systray.build", env: env, :live_stream => Omnibus.logger.live_stream(:info)
  else
    command "dda inv -- -e rtloader.clean", :live_stream => Omnibus.logger.live_stream(:info)
    command "dda inv -- -e rtloader.make --install-prefix \"#{install_dir}/embedded\" --cmake-options '-DCMAKE_CXX_FLAGS:=\"-D_GLIBCXX_USE_CXX11_ABI=0\" -DCMAKE_INSTALL_LIBDIR=lib -DCMAKE_FIND_FRAMEWORK:STRING=NEVER -DPython3_EXECUTABLE=#{install_dir}/embedded/bin/python3'", :env => env, :live_stream => Omnibus.logger.live_stream(:info)
    command "dda inv -- -e rtloader.install", :live_stream => Omnibus.logger.live_stream(:info)

    command "dda inv -- -e agent.build --exclude-rtloader --no-development --install-path=#{install_dir} --embedded-path=#{install_dir}/embedded --flavor #{flavor_arg}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
  end

  if osx_target?
    conf_dir = "#{install_dir}/etc"
  else
    conf_dir = "#{install_dir}/etc/datadog-agent"
  end
  mkdir conf_dir
  mkdir "#{install_dir}/bin"
  unless windows_target?
    mkdir "#{install_dir}/run/"
    mkdir "#{install_dir}/scripts/"
  end

  # move around bin and config files
  move 'bin/agent/dist/datadog.yaml', "#{conf_dir}/datadog.yaml.example"
  copy 'bin/agent/dist/conf.d/.', "#{conf_dir}"
  delete 'bin/agent/dist/conf.d'

  unless windows_target?
    copy 'bin/agent', "#{install_dir}/bin/"
  else
    copy 'bin/agent/ddtray.exe', "#{install_dir}/bin/agent"
    copy 'bin/agent/agent.exe', "#{install_dir}/bin/agent"
    copy 'bin/agent/dist', "#{install_dir}/bin/agent"
    mkdir "#{install_dir}/bin/scripts/"
    copy "#{project_dir}/omnibus/windows-scripts/iis-instrumentation.bat", "#{install_dir}/bin/scripts/"
    copy "#{project_dir}/omnibus/windows-scripts/host-instrumentation.bat", "#{install_dir}/bin/scripts/"
    mkdir Omnibus::Config.package_dir() unless Dir.exists?(Omnibus::Config.package_dir())
  end

  platform = windows_arch_i386? ? "x86" : "x64"
  command "dda inv -- -e trace-agent.build --install-path=#{install_dir} --flavor #{flavor_arg}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)

  # Build the installer
  # We do this in the same software definition to avoid redundant copying, as it's based on the same source
  if linux_target? and !heroku_target?
    command "invoke installer.build #{fips_args} --no-cgo --run-path=/opt/datadog-packages/run --install-path=#{install_dir}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    move 'bin/installer/installer', "#{install_dir}/embedded/bin"
  elsif windows_target?
    command "dda inv -- -e installer.build #{fips_args} --install-path=#{install_dir}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    move 'bin/installer/installer.exe', "#{install_dir}/datadog-installer.exe"
  end

  if linux_target?
    if heroku_target?
      # shouldn't be needed in practice, but it is used by the systemd service,
      # which is used when installing the deb manually
      copy "cmd/loader/main_noop.sh", "#{install_dir}/embedded/bin/trace-loader"
      command "chmod 0755 #{install_dir}/embedded/bin/trace-loader"
    else
      command "dda inv -- -e loader.build --install-path=#{install_dir}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)
      copy "bin/trace-loader/trace-loader", "#{install_dir}/embedded/bin"
    end
  end

  if windows_target?
    copy 'bin/trace-agent/trace-agent.exe', "#{install_dir}/bin/agent"
  else
    copy 'bin/trace-agent/trace-agent', "#{install_dir}/embedded/bin"
  end

  # Process agent
  if not heroku_target?
    command "dda inv -- -e process-agent.build --install-path=#{install_dir} --flavor #{flavor_arg}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)
  end

  if windows_target?
    copy 'bin/process-agent/process-agent.exe', "#{install_dir}/bin/agent"
  elsif not heroku_target?
    copy 'bin/process-agent/process-agent', "#{install_dir}/embedded/bin"
  end

  # Private action runner
  if not heroku_target? and not fips_mode?
    command "dda inv -- -e privateactionrunner.build --install-path=#{install_dir} --flavor #{flavor_arg}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)

    if windows_target?
      copy 'bin/privateactionrunner/privateactionrunner.exe', "#{install_dir}/bin/agent"
    elsif not heroku_target?
      copy 'bin/privateactionrunner/privateactionrunner', "#{install_dir}/embedded/bin"
    end
  end

  # System-probe
  if sysprobe_enabled? || osx_target? || (windows_target? && do_windows_sysprobe != "")
    if linux_target?
      command "dda inv -- -e system-probe.build-sysprobe-binary #{fips_args} --install-path=#{install_dir}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
      command "!(objdump -p ./bin/system-probe/system-probe | egrep 'GLIBC_2\.(1[8-9]|[2-9][0-9])')"
    else
      command "dda inv -- -e system-probe.build #{fips_args}", env: env, :live_stream => Omnibus.logger.live_stream(:info)
    end

    if windows_target?
      copy 'bin/system-probe/system-probe.exe', "#{install_dir}/bin/agent"
    else
      copy "bin/system-probe/system-probe", "#{install_dir}/embedded/bin"
    end

    # Add SELinux policy for system-probe
    if debian_target? || redhat_target?
      mkdir "#{conf_dir}/selinux"
      command "dda inv -- -e selinux.compile-system-probe-policy-file --output-directory #{conf_dir}/selinux", env: env
    end

    move 'bin/agent/dist/system-probe.yaml', "#{conf_dir}/system-probe.yaml.example"
  end

  # System-probe eBPF files
  if sysprobe_enabled?
    mkdir "#{install_dir}/embedded/share/system-probe/ebpf"
    mkdir "#{install_dir}/embedded/share/system-probe/ebpf/runtime"
    mkdir "#{install_dir}/embedded/share/system-probe/ebpf/co-re"
    mkdir "#{install_dir}/embedded/share/system-probe/ebpf/co-re/btf"

    arch = `uname -m`.strip
    if arch == "aarch64"
      arch = "arm64"
    end
    copy "pkg/ebpf/bytecode/build/#{arch}/*.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    delete "#{install_dir}/embedded/share/system-probe/ebpf/usm_events_test*.o"
    copy "pkg/ebpf/bytecode/build/#{arch}/co-re/*.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/"
    copy "pkg/ebpf/bytecode/build/runtime/*.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/clang-bpf", "#{install_dir}/embedded/bin/clang-bpf"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/llc-bpf", "#{install_dir}/embedded/bin/llc-bpf"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/minimized-btfs.tar.xz", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/btf/minimized-btfs.tar.xz"

    copy 'pkg/ebpf/c/COPYING', "#{install_dir}/embedded/share/system-probe/ebpf/"

  end

  # sd-agent (service discovery agent)
  if ENV['SD_AGENT_BIN'] && linux_target?
    copy ENV['SD_AGENT_BIN'], "#{install_dir}/embedded/bin/sd-agent"
  end

  # dd-procmgrd (process manager daemon)
  if ENV['WITH_DD_PROCMGRD'] == 'true'
    command_on_repo_root "bazel run --config=dd-procmgrd-release //pkg/procmgr/rust:install -- --destdir=#{install_dir}/embedded/bin", :env => env, :live_stream => Omnibus.logger.live_stream(:info)
  end

  # Security agent
  unless heroku_target?
    command "dda inv -- -e security-agent.build #{fips_args} --install-path=#{install_dir}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)
    if windows_target?
      copy 'bin/security-agent/security-agent.exe', "#{install_dir}/bin/agent"
    else
      copy 'bin/security-agent/security-agent', "#{install_dir}/embedded/bin"
    end
    move 'bin/agent/dist/security-agent.yaml', "#{conf_dir}/security-agent.yaml.example"
  end

  # CWS Instrumentation
  cws_inst_support = !heroku_target? && linux_target?
  if cws_inst_support
    command "dda inv -- -e cws-instrumentation.build #{fips_args}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)
    copy 'bin/cws-instrumentation/cws-instrumentation', "#{install_dir}/embedded/bin"
  end

# Secret Generic Connector
  if !heroku_target?
    command "dda inv -- -e secret-generic-connector.build #{fips_args}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)
    if windows_target?
      copy 'bin/secret-generic-connector/secret-generic-connector.exe', "#{install_dir}/bin/agent"
    else
      copy 'bin/secret-generic-connector/secret-generic-connector', "#{install_dir}/embedded/bin"
    end
    mkdir "#{install_dir}/LICENSES"
    copy 'cmd/secret-generic-connector/LICENSE', "#{install_dir}/LICENSES/secret-generic-connector-LICENSE"
  end

  if osx_target?
    # Launchd service definition
    erb source: "launchd.plist.example.erb",
        dest: "#{conf_dir}/com.datadoghq.agent.plist.example",
        mode: 0644,
        vars: { install_dir: install_dir }

    erb source: "launchd.sysprobe.plist.example.erb",
        dest: "#{conf_dir}/com.datadoghq.sysprobe.plist.example",
        mode: 0644,
        vars: {
          # Due to how install_dir actually matches where the Agent is built rather than
          # its actual final destination, we hardcode here the currently sole supported install location
          install_dir: "/opt/datadog-agent",
          conf_dir: "/opt/datadog-agent/etc",
        }

    erb source: "gui.launchd.plist.erb",
        dest: "#{conf_dir}/com.datadoghq.gui.plist.example",
        mode: 0644,
        vars: {
          # Due to how install_dir actually matches where the Agent is built rather than
          # its actual final destination, we hardcode here the currently sole supported install location
          install_dir: "/opt/datadog-agent",
        }

    # Systray GUI
    app_temp_dir = "#{install_dir}/Datadog Agent.app/Contents"
    mkdir "#{app_temp_dir}/MacOS"
    systray_build_dir = "#{project_dir}/comp/core/gui/guiimpl/systray"
    # Add @executable_path/../Frameworks to rpath to find the swift libs in the Frameworks folder.
    target = "#{arm_target? ? 'arm64' : 'x86_64'}-apple-macos11.0" # https://docs.datadoghq.com/agent/supported_platforms/?tab=macos
    command "swiftc -O -swift-version \"5\" -target \"#{target}\" -Xlinker '-rpath' -Xlinker '@executable_path/../Frameworks' Sources/*.swift -o gui", cwd: systray_build_dir
    copy "#{systray_build_dir}/gui", "#{app_temp_dir}/MacOS/"
    copy "#{systray_build_dir}/agent.png", "#{app_temp_dir}/MacOS/"
  end

  # APM Hands Off config file
  if linux_target?
    copy 'pkg/config/example/application_monitoring.yaml.example', "#{conf_dir}/application_monitoring.yaml.example"
  end

  # Allows the agent to be installed in a custom location
  if linux_target?
    command "touch #{install_dir}/.install_root"
  end

  if fips_mode? && linux_target?
    # Put the ruby code in a block to prevent omnibus from running it directly
    # but rather at build step with the rest of the code above.
    # If not in a block, it will search for binaries that have not been built yet.
    block do
      LINUX_BINARIES = [
        "bin/agent/agent",
        "embedded/bin/trace-agent",
        "embedded/bin/process-agent",
        "embedded/bin/security-agent",
        "embedded/bin/system-probe",
        "embedded/bin/installer",
        "embedded/bin/secret-generic-connector",
      ]

      LINUX_BINARIES.each do |bin|
        fips_check_binary_for_expected_symbol(File.join(install_dir, bin))
      end
    end
  end

  block do
    python_scripts_dir = "#{project_dir}/omnibus/python-scripts"
    mkdir "#{install_dir}/python-scripts"
    Dir.glob("#{python_scripts_dir}/*").each do |file|
      unless File.basename(file).end_with?('_tests.py')
        copy file, "#{install_dir}/python-scripts"
      end
    end
  end
end
