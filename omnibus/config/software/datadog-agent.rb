# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'pathname'

name 'datadog-agent'

# creates required build directories
dependency 'datadog-agent-prepare'

dependency "python3"

dependency "openscap" if linux_target? and !arm7l_target? and !heroku_target? # Security-agent dependency, not needed for Heroku

# Alternative memory allocator which has better support for memory allocated by cgo calls,
# especially at higher thread counts.
dependency "libjemalloc" if linux_target?

dependency 'agent-dependencies'
dependency 'datadog-agent-dependencies'

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

always_build true

build do
  license :project_license

  bundled_agents = []
  if heroku_target?
    bundled_agents = ["process-agent"]
  end

  # set GOPATH on the omnibus source dir for this software
  gopath = Pathname.new(project_dir) + '../../../..'
  flavor_arg = ENV['AGENT_FLAVOR']
  fips_arg = fips_mode? ? "--fips-mode" : ""
  if windows_target?
    env = {
        'GOPATH' => gopath.to_path,
        'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
    }
    major_version_arg = "%MAJOR_VERSION%"
  else
    env = {
        'GOPATH' => gopath.to_path,
        'PATH' => "#{gopath.to_path}/bin:#{ENV['PATH']}",
        "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
        "CGO_CFLAGS" => "-I. -I#{install_dir}/embedded/include",
        "CGO_LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib"
    }
    major_version_arg = "$MAJOR_VERSION"
  end

  unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
    gomodcache = Pathname.new(ENV["OMNIBUS_GOMODCACHE"])
    env["GOMODCACHE"] = gomodcache.to_path
  end

  # include embedded path (mostly for `pkg-config` binary)
  env = with_standard_compiler_flags(with_embedded_path(env))
  default_install_dir = "/opt/datadog-agent"
  if Omnibus::Config.host_distribution == "ociru"
    default_install_dir = "#{install_dir}"
  end

  # we assume the go deps are already installed before running omnibus
  if windows_target?
    platform = windows_arch_i386? ? "x86" : "x64"
    do_windows_sysprobe = ""
    if not windows_arch_i386? and ENV['WINDOWS_DDNPM_DRIVER'] and not ENV['WINDOWS_DDNPM_DRIVER'].empty?
      do_windows_sysprobe = "--windows-sysprobe"
    end
    command "inv -e rtloader.clean"
    command "inv -e rtloader.make --install-prefix \"#{windows_safe_path(python_2_embedded)}\" --cmake-options \"-G \\\"Unix Makefiles\\\" \\\"-DPython3_EXECUTABLE=#{windows_safe_path(python_3_embedded)}\\python.exe\"\"", :env => env
    command "mv rtloader/bin/*.dll  #{install_dir}/bin/agent/"
    command "inv -e agent.build --exclude-rtloader --major-version #{major_version_arg} --rebuild --no-development --install-path=#{install_dir} --embedded-path=#{install_dir}/embedded #{do_windows_sysprobe} --flavor #{flavor_arg}", env: env
    command "inv -e systray.build --major-version #{major_version_arg} --rebuild", env: env
  else
    command "inv -e rtloader.clean"
    command "inv -e rtloader.make --install-prefix \"#{install_dir}/embedded\" --cmake-options '-DCMAKE_CXX_FLAGS:=\"-D_GLIBCXX_USE_CXX11_ABI=0\" -DCMAKE_INSTALL_LIBDIR=lib -DCMAKE_FIND_FRAMEWORK:STRING=NEVER -DPython3_EXECUTABLE=#{install_dir}/embedded/bin/python3'", :env => env
    command "inv -e rtloader.install"
    bundle_arg = bundled_agents ? bundled_agents.map { |k| "--bundle #{k}" }.join(" ") : "--bundle agent"

    include_sds = ""
    if linux_target?
        include_sds = "--include-sds" # we only support SDS on Linux targets for now
    end
    command "inv -e agent.build --exclude-rtloader #{include_sds} --major-version #{major_version_arg} --rebuild --no-development --install-path=#{install_dir} --embedded-path=#{default_install_dir}/embedded --python-home-2=#{default_install_dir}/embedded --python-home-3=#{default_install_dir}/embedded --flavor #{flavor_arg} #{bundle_arg}", env: env

    if heroku_target?
      command "inv -e agent.build --exclude-rtloader --major-version #{major_version_arg} --rebuild --no-development --install-path=#{install_dir} --embedded-path=#{install_dir}/embedded --python-home-2=#{install_dir}/embedded --python-home-3=#{install_dir}/embedded --flavor #{flavor_arg} --agent-bin=bin/agent/core-agent --bundle agent", env: env
    end
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
  move 'bin/agent/dist/conf.d', "#{conf_dir}/"

  unless windows_target?
    copy 'bin/agent', "#{install_dir}/bin/"
  else
    copy 'bin/agent/ddtray.exe', "#{install_dir}/bin/agent"
    copy 'bin/agent/agent.exe', "#{install_dir}/bin/agent"
    copy 'bin/agent/dist', "#{install_dir}/bin/agent"
    mkdir Omnibus::Config.package_dir() unless Dir.exists?(Omnibus::Config.package_dir())
  end

  if not bundled_agents.include? "trace-agent"
    platform = windows_arch_i386? ? "x86" : "x64"
    command "invoke trace-agent.build --install-path=#{install_dir} --major-version #{major_version_arg} --flavor #{flavor_arg}", :env => env
  end

  if windows_target?
    copy 'bin/trace-agent/trace-agent.exe', "#{install_dir}/bin/agent"
  else
    copy 'bin/trace-agent/trace-agent', "#{install_dir}/embedded/bin"
  end

  # Process agent
  if not bundled_agents.include? "process-agent"
    command "invoke -e process-agent.build --install-path=#{install_dir} --major-version #{major_version_arg} --flavor #{flavor_arg} --no-bundle", :env => env
  end

  if windows_target?
    copy 'bin/process-agent/process-agent.exe', "#{install_dir}/bin/agent"
  else
    copy 'bin/process-agent/process-agent', "#{install_dir}/embedded/bin"
  end

  # System-probe
  sysprobe_support = (not heroku_target?) && (linux_target? || (windows_target? && do_windows_sysprobe != ""))
  if sysprobe_support
    if not bundled_agents.include? "system-probe"
      if windows_target?
        command "invoke -e system-probe.build #{fips_arg}", env: env
      elsif linux_target?
        command "invoke -e system-probe.build-sysprobe-binary #{fips_arg} --install-path=#{install_dir} --no-bundle", env: env
      end
    end

    if windows_target?
      copy 'bin/system-probe/system-probe.exe', "#{install_dir}/bin/agent"
    elsif linux_target?
      copy "bin/system-probe/system-probe", "#{install_dir}/embedded/bin"
    end

    # Add SELinux policy for system-probe
    if debian_target? || redhat_target?
      mkdir "#{conf_dir}/selinux"
      command "inv -e selinux.compile-system-probe-policy-file --output-directory #{conf_dir}/selinux", env: env
    end

    move 'bin/agent/dist/system-probe.yaml', "#{conf_dir}/system-probe.yaml.example"
  end

  # Security agent
  secagent_support = (not heroku_target?) and (not windows_target? or (ENV['WINDOWS_DDPROCMON_DRIVER'] and not ENV['WINDOWS_DDPROCMON_DRIVER'].empty?))
  if secagent_support
    if not bundled_agents.include? "security-agent"
      command "invoke -e security-agent.build #{fips_arg} --install-path=#{install_dir} --major-version #{major_version_arg} --no-bundle", :env => env
    end
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
    command "invoke -e cws-instrumentation.build", :env => env
    copy 'bin/cws-instrumentation/cws-instrumentation', "#{install_dir}/embedded/bin"
  end

  # OTel agent - can never be bundled
  if ot_target?
    unless windows_target?
      command "invoke -e otel-agent.build", :env => env
      copy 'bin/otel-agent/otel-agent', "#{install_dir}/embedded/bin"

      move 'bin/otel-agent/dist/otel-config.yaml', "#{conf_dir}/otel-config.yaml.example"
    end
  end

  # APM Injection agent
  if windows_target?
    if ENV['WINDOWS_APMINJECT_MODULE'] and not ENV['WINDOWS_APMINJECT_MODULE'].empty?
      command "inv agent.generate-config --build-type apm-injection --output-file ./bin/agent/dist/apm-inject.yaml", :env => env
      move 'bin/agent/dist/apm-inject.yaml', "#{conf_dir}/apm-inject.yaml.example"
    end
  end

  if osx_target?
    # Launchd service definition
    erb source: "launchd.plist.example.erb",
        dest: "#{conf_dir}/com.datadoghq.agent.plist.example",
        mode: 0644,
        vars: { install_dir: install_dir }

    erb source: "gui.launchd.plist.erb",
        dest: "#{conf_dir}/com.datadoghq.gui.plist.example",
        mode: 0644

    # Systray GUI
    app_temp_dir = "#{install_dir}/Datadog Agent.app/Contents"
    mkdir "#{app_temp_dir}/MacOS"
    systray_build_dir = "#{project_dir}/comp/core/gui/guiimpl/systray"
    # Target OSX 10.10 (it brings significant changes to Cocoa and Foundation APIs, and older versions of OSX are EOL'ed)
    # Add @executable_path/../Frameworks to rpath to find the swift libs in the Frameworks folder.
    command 'swiftc -O -swift-version "5" -target "x86_64-apple-macosx10.10" -Xlinker \'-rpath\' -Xlinker \'@executable_path/../Frameworks\' Sources/*.swift -o gui', cwd: systray_build_dir
    copy "#{systray_build_dir}/gui", "#{app_temp_dir}/MacOS/"
    copy "#{systray_build_dir}/agent.png", "#{app_temp_dir}/MacOS/"
  end

  # The file below is touched by software builds that don't put anything in the installation
  # directory (libgcc right now) so that the git_cache gets updated let's remove it from the
  # final package
  # Change RPATH from the install_dir to relative RPATH
  unless windows_target?
    delete "#{install_dir}/uselessfile"
  end

  # TODO: move this to omnibus-ruby::health-check.rb
  # check that linux binaries contains OpenSSL symbols when building to support FIPS
  if fips_mode? && linux_target?
      # Put the ruby code in a block to prevent omnibus from running it directly but rather at build step with the rest of the code above.
      # If not in a block, it will search for binaries that have not been built yet.
      block do
        LINUX_BINARIES = [
          "#{install_dir}/bin/agent/agent",
          "#{install_dir}/embedded/bin/trace-agent",
          "#{install_dir}/embedded/bin/process-agent",
          "#{install_dir}/embedded/bin/security-agent",
          "#{install_dir}/embedded/bin/system-probe",
        ]

        symbol = "_Cfunc_go_openssl"
        check_block = Proc.new { |binary, symbols|
          count = symbols.scan(symbol).count
          if count > 0
            log.info(log_key) { "Symbol '#{symbol}' found #{count} times in binary '#{binary}'." }
          else
            raise FIPSSymbolsNotFound.new("Expected to find '#{symbol}' symbol in #{binary} but did not")
          end
        }.curry

        LINUX_BINARIES.each do |bin|
          partially_applied_check = check_block.call(bin)
          GoSymbolsInspector.new(bin,  &partially_applied_check).inspect()
        end
      end
  end


  python_scripts_dir = "#{project_dir}/omnibus/python-scripts"
  mkdir "#{install_dir}/python-scripts"
  copy "#{python_scripts_dir}/*", "#{install_dir}/python-scripts"
end
