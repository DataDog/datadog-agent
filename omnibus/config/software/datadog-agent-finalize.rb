# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-agent-finalize"
description "steps required to finalize the build"
default_version "1.0.0"

skip_transitive_dependency_licensing true


always_build true

build do
    license :project_license

    output_config_dir = ENV["OUTPUT_CONFIG_DIR"]
    flavor_arg = ENV['AGENT_FLAVOR']
    # TODO too many things done here, should be split
    block do
        # Conf files
        if windows_target?
            conf_dir_root = "#{Omnibus::Config.source_dir()}/etc/datadog-agent"
            conf_dir = "#{conf_dir_root}/extra_package_files/EXAMPLECONFSLOCATION"
            # delete the directory if it exists, then recreate it, because
            # move will skip/silently fail if the destination directory exists
            delete conf_dir
            mkdir conf_dir
            move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", conf_dir_root, :force=>true
            if ENV['WINDOWS_DDNPM_DRIVER'] and not ENV['WINDOWS_DDNPM_DRIVER'].empty? and not windows_arch_i386?
              move "#{install_dir}/etc/datadog-agent/system-probe.yaml.example", conf_dir_root, :force=>true
            end
            if ENV['WINDOWS_DDPROCMON_DRIVER'] and not ENV['WINDOWS_DDPROCMON_DRIVER'].empty? and not windows_arch_i386?
              move "#{install_dir}/etc/datadog-agent/security-agent.yaml.example", conf_dir_root, :force=>true
              move "#{install_dir}/etc/datadog-agent/runtime-security.d", conf_dir_root, :force=>true
              move "#{conf_dir_root}/runtime-security.d/default.policy", "#{conf_dir_root}/runtime-security.d/default.policy.example", :force=>true
            end
            if ENV['WINDOWS_APMINJECT_MODULE'] and not ENV['WINDOWS_APMINJECT_MODULE'].empty?
              move "#{install_dir}/etc/datadog-agent/apm-inject.yaml.example", conf_dir_root, :force=>true
            end
            move "#{install_dir}/etc/datadog-agent/conf.d/*", conf_dir, :force=>true

            # remove the config files for the subservices; they'll be started
            # based on the config file
            delete "#{conf_dir}/apm.yaml.default"
            delete "#{conf_dir}/process_agent.yaml.default"

            # load isn't supported by windows
            delete "#{conf_dir}/load.d"

            # service_discovery isn't supported by windows
            delete "#{conf_dir}/service_discovery.d"

            # Remove .pyc files from embedded Python
            command "del /q /s #{windows_safe_path(install_dir)}\\*.pyc"
        end

        if linux_target? || osx_target?
            # Setup script aliases, e.g. `/opt/datadog-agent/embedded/bin/pip` will
            # default to `pip2` if the default Python runtime is Python 2.
            delete "#{install_dir}/embedded/bin/pip"
            link "#{install_dir}/embedded/bin/pip3", "#{install_dir}/embedded/bin/pip"

            delete "#{install_dir}/embedded/bin/python"
            link "#{install_dir}/embedded/bin/python3", "#{install_dir}/embedded/bin/python"

            # Used in https://docs.datadoghq.com/agent/guide/python-3/
            delete "#{install_dir}/embedded/bin/2to3"
            link "#{install_dir}/embedded/bin/2to3-3.12", "#{install_dir}/embedded/bin/2to3"

            delete "#{install_dir}/embedded/lib/config_guess"

            # Delete .pc files which aren't needed after building
            delete "#{install_dir}/embedded/lib/pkgconfig"
            # Same goes for .cmake files
            delete "#{install_dir}/embedded/lib/cmake"
            # and for libtool files
            delete "#{install_dir}/embedded/lib/*.la"
        end

        if linux_target?
            # Move configuration files
            mkdir "#{output_config_dir}/etc/datadog-agent"
            move "#{install_dir}/bin/agent/dd-agent", "/usr/bin/dd-agent"
            move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", "#{output_config_dir}/etc/datadog-agent"
            move "#{install_dir}/etc/datadog-agent/conf.d", "#{output_config_dir}/etc/datadog-agent", :force=>true
            unless heroku_target?
              move "#{install_dir}/etc/datadog-agent/system-probe.yaml.example", "#{output_config_dir}/etc/datadog-agent"
              move "#{install_dir}/etc/datadog-agent/security-agent.yaml.example", "#{output_config_dir}/etc/datadog-agent", :force=>true
              move "#{install_dir}/etc/datadog-agent/runtime-security.d", "#{output_config_dir}/etc/datadog-agent", :force=>true
              move "#{install_dir}/etc/datadog-agent/compliance.d", "#{output_config_dir}/etc/datadog-agent"

              # Move SELinux policy
              if debian_target? || redhat_target?
                move "#{install_dir}/etc/datadog-agent/selinux", "#{output_config_dir}/etc/datadog-agent/selinux"
              end
            end

            if ot_target?
              move "#{install_dir}/etc/datadog-agent/otel-config.yaml.example", "#{output_config_dir}/etc/datadog-agent"
            end

            # Create empty directories so that they're owned by the package
            # (also requires `extra_package_file` directive in project def)
            mkdir "#{output_config_dir}/etc/datadog-agent/checks.d"
            mkdir "/var/log/datadog"

            # remove unused configs
            delete "#{output_config_dir}/etc/datadog-agent/conf.d/apm.yaml.default"
            delete "#{output_config_dir}/etc/datadog-agent/conf.d/process_agent.yaml.default"

            # remove windows specific configs
            delete "#{output_config_dir}/etc/datadog-agent/conf.d/winproc.d"

            # cleanup clutter
            delete "#{install_dir}/etc"

            # The prerm script of the package should use this list to remove the pyc/pyo files
            command "echo '# DO NOT REMOVE/MODIFY - used by package removal tasks' > #{install_dir}/embedded/.py_compiled_files.txt"
            command "find #{install_dir}/embedded '(' -name '*.pyc' -o -name '*.pyo' ')' -type f -delete -print >> #{install_dir}/embedded/.py_compiled_files.txt"

            # The prerm and preinst scripts of the package will use this list to detect which files
            # have been setup by the installer, this way, on removal, we'll be able to delete only files
            # which have not been created by the package.
            command "echo '# DO NOT REMOVE/MODIFY - used by package removal tasks' > #{install_dir}/embedded/.installed_by_pkg.txt"
            command "find . -path './embedded/lib/python*/site-packages/*' >> #{install_dir}/embedded/.installed_by_pkg.txt", cwd: install_dir

            # removing the doc from the embedded folder to reduce package size by ~3MB
            delete "#{install_dir}/embedded/share/doc"

            # removing the terminfo db from the embedded folder to reduce package size by ~7MB
            delete "#{install_dir}/embedded/share/terminfo"
            # removing the symlink too
            delete "#{install_dir}/embedded/lib/terminfo"

            # removing useless folder
            delete "#{install_dir}/embedded/share/aclocal"
            delete "#{install_dir}/embedded/share/examples"

            # removing the man pages from the embedded folder to reduce package size by ~4MB
            delete "#{install_dir}/embedded/man"
            delete "#{install_dir}/embedded/share/man"

            # linux build will be stripped - but psycopg2 affected by bug in the way binutils
            # and patchelf work together:
            #    https://github.com/pypa/manylinux/issues/119
            #    https://github.com/NixOS/patchelf
            #
            # Only affects psycopg2 - any binary whose path matches the pattern will be
            # skipped.
            strip_exclude("*psycopg2*")
            strip_exclude("*cffi_backend*")

            # We get the following error when the aerospike lib is stripped:
            # The `aerospike` client is not installed: /opt/datadog-agent/embedded/lib/python2.7/site-packages/aerospike.so: ELF load command address/offset not properly aligned
            strip_exclude("*aerospike*")

            # Do not strip eBPF programs
            strip_exclude("#{install_dir}/embedded/share/system-probe/ebpf/*.o")
            strip_exclude("#{install_dir}/embedded/share/system-probe/ebpf/co-re/*.o")

            # Most postgres binaries are removed in postgres' own software
            # recipe, but we need pg_config to build psycopq.
            delete "#{install_dir}/embedded/bin/pg_config"

            # Edit rpath from a true path to relative path for each binary
            command "inv omnibus.rpath-edit #{install_dir} #{install_dir}", cwd: Dir.pwd
        end

        if osx_target?
            # Remove linux specific configs
            delete "#{install_dir}/etc/conf.d/file_handle.d"

            # remove windows specific configs
            delete "#{install_dir}/etc/conf.d/winproc.d"

            # remove docker configuration
            delete "#{install_dir}/etc/conf.d/docker.d"

            # Edit rpath from a true path to relative path for each binary
            command "inv omnibus.rpath-edit #{install_dir} #{install_dir} --platform=macos", cwd: Dir.pwd

            if ENV['HARDENED_RUNTIME_MAC'] == 'true'
                hardened_runtime = "-o runtime --entitlements #{entitlements_file} "
            else
                hardened_runtime = ""
            end

            if code_signing_identity
                # Codesign everything
                command "find #{install_dir} -type f | grep -E '(\\.so|\\.dylib)' | xargs -I{} codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}' '{}'"
                command "find #{install_dir}/embedded/bin -perm +111 -type f | xargs -I{} codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}' '{}'"
                command "find #{install_dir}/embedded/sbin -perm +111 -type f | xargs -I{} codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}' '{}'"
                command "find #{install_dir}/bin -perm +111 -type f | xargs -I{} codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}' '{}'"
                command "codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}' '#{install_dir}/Datadog Agent.app'"
            end
        end
    end
end
