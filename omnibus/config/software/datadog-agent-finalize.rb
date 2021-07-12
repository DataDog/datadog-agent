# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-agent-finalize"
description "steps required to finalize the build"
default_version "1.0.0"
skip_transitive_dependency_licensing true

build do
    # TODO too many things done here, should be split
    block do
        # Conf files
        if windows?
            conf_dir_root = "#{Omnibus::Config.source_dir()}/etc/stackstate-agent"
            conf_dir = "#{conf_dir_root}/extra_package_files/EXAMPLECONFSLOCATION"
            mkdir conf_dir
            move "#{install_dir}/etc/stackstate-agent/stackstate.yaml.example", conf_dir_root, :force=>true
            move "#{install_dir}/etc/stackstate-agent/conf.d/*", conf_dir, :force=>true
            delete "#{install_dir}/bin/agent/agent.exe"
            # TODO why does this get generated at all
            delete "#{install_dir}/bin/agent/agent.exe~"

            #remove unneccessary copies caused by blanked copy of bin to #{install_dir} in datadog-agent recipe
            delete "#{install_dir}/bin/agent/libdatadog-agent-three.dll"
            delete "#{install_dir}/bin/agent/libdatadog-agent-two.dll"
            delete "#{install_dir}/bin/agent/customaction.dll"

            # not sure where it's coming from, but we're being left with an `embedded` dir.
            # delete it
            delete "#{install_dir}/embedded"

            # remove the config files for the subservices; they'll be started
            # based on the config file
            delete "#{conf_dir}/apm.yaml.default"
            delete "#{conf_dir}/process_agent.yaml.default"
            # load isn't supported by windows
            delete "#{conf_dir}/load.d"
            # disk isn't supported by windows
            delete "#{conf_dir}/disk.d"
            # docker isn't supported by windows
            delete "#{conf_dir}/docker.d"
            # docker swarm isn't supported by windows
            delete "#{conf_dir}/docker_swarm.d"

            # cleanup clutter
            delete "#{install_dir}/etc"
            delete "#{install_dir}/bin/agent/dist/conf.d"
            delete "#{install_dir}/bin/agent/dist/*.conf*"
            delete "#{install_dir}/bin/agent/dist/*.yaml"
            command "del /q /s #{windows_safe_path(install_dir)}\\*.pyc"
        elsif linux?
            # Fix pip after building on extended toolchain in CentOS builder
            if redhat?
                unless arm?
                    rhel_toolchain_root = "/opt/centos/devtoolset-1.1/root"
                    # lets be cautious - we first search for the expected toolchain path, if its not there, bail out
                    command "find #{install_dir} -type f -iname '*_sysconfigdata*.py' -exec grep -inH '#{rhel_toolchain_root}' {} \\; |  egrep '.*'"
                    # replace paths with expected target toolchain location
                    command "find #{install_dir} -type f -iname '*_sysconfigdata*.py' -exec sed -i 's##{rhel_toolchain_root}##g' {} \\;"
                end
            end

            # Move system service files
            mkdir "/etc/init"
            move "#{install_dir}/scripts/stackstate-agent.conf", "/etc/init"
            move "#{install_dir}/scripts/stackstate-agent-trace.conf", "/etc/init"
            move "#{install_dir}/scripts/stackstate-agent-process.conf", "/etc/init"
            move "#{install_dir}/scripts/stackstate-agent-sysprobe.conf", "/etc/init"
            if $enable_security_agent
                move "#{install_dir}/scripts/stackstate-agent-security.conf", "/etc/init"
            end
            systemd_directory = "/usr/lib/systemd/system"
            if debian?
                # debian recommends using a different directory for systemd unit files
                systemd_directory = "/lib/systemd/system"

                # sysvinit support for debian only for now
                mkdir "/etc/init.d"
                move "#{install_dir}/scripts/stackstate-agent", "/etc/init.d"
                move "#{install_dir}/scripts/stackstate-agent-trace", "/etc/init.d"
                move "#{install_dir}/scripts/stackstate-agent-process", "/etc/init.d"
                if $enable_security_agent
                    move "#{install_dir}/scripts/stackstate-agent-security", "/etc/init.d"
                end
            end
            if suse?
                mkdir "/etc/init.d"
                move "#{install_dir}/scripts/stackstate-agent", "/etc/init.d"
                move "#{install_dir}/scripts/stackstate-agent-trace", "/etc/init.d"
                move "#{install_dir}/scripts/stackstate-agent-process", "/etc/init.d"
                if $enable_security_agent
                    move "#{install_dir}/scripts/stackstate-agent-security", "/etc/init.d"
                end
            end
            mkdir systemd_directory
            move "#{install_dir}/scripts/stackstate-agent.service", systemd_directory
            move "#{install_dir}/scripts/stackstate-agent-trace.service", systemd_directory
            move "#{install_dir}/scripts/stackstate-agent-process.service", systemd_directory
            move "#{install_dir}/scripts/stackstate-agent-sysprobe.service", systemd_directory
            if $enable_security_agent
                move "#{install_dir}/scripts/stackstate-agent-security.service", systemd_directory
            end

            # Move configuration files
            mkdir "/etc/stackstate-agent"
            move "#{install_dir}/bin/agent/sts-agent", "/usr/bin/sts-agent"
            move "#{install_dir}/etc/stackstate-agent/stackstate.yaml.example", "/etc/stackstate-agent"
            move "#{install_dir}/etc/stackstate-agent/system-probe.yaml.example", "/etc/stackstate-agent"
            move "#{install_dir}/etc/stackstate-agent/conf.d", "/etc/stackstate-agent", :force=>true

            # Move SELinux policy
            if debian? || redhat?
                move "#{install_dir}/etc/datadog-agent/selinux", "/etc/datadog-agent/selinux"
            end

            # Create empty directories so that they're owned by the package
            # (also requires `extra_package_file` directive in project def)
            mkdir "/etc/stackstate-agent/checks.d"
            mkdir "/var/log/stackstate-agent"

            # remove unused configs
            delete "/etc/stackstate-agent/conf.d/apm.yaml.default"
            delete "/etc/stackstate-agent/conf.d/process_agent.yaml.default"

            # remove windows specific configs
            delete "/etc/stackstate-agent/conf.d/winproc.d"

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

            # Setup script aliases, e.g. `/opt/datadog-agent/embedded/bin/pip` will
            # default to `pip2` if the default Python runtime is Python 2.
            if with_python_runtime? "2"
                delete "#{install_dir}/embedded/bin/pip"
                link "#{install_dir}/embedded/bin/pip2", "#{install_dir}/embedded/bin/pip"

                delete "#{install_dir}/embedded/bin/2to3"
                link "#{install_dir}/embedded/bin/2to3-2.7", "#{install_dir}/embedded/bin/2to3"
                # Setup script aliases, e.g. `/opt/datadog-agent/embedded/bin/pip` will
                # default to `pip3` if the default Python runtime is Python 3 (Agent 7.x).
                # Caution: we don't want to do this for Agent 6.x
            elsif with_python_runtime? "3"
                delete "#{install_dir}/embedded/bin/pip"
                link "#{install_dir}/embedded/bin/pip3", "#{install_dir}/embedded/bin/pip"

                delete "#{install_dir}/embedded/bin/python"
                link "#{install_dir}/embedded/bin/python3", "#{install_dir}/embedded/bin/python"

                delete "#{install_dir}/embedded/bin/2to3"
                link "#{install_dir}/embedded/bin/2to3-3.7", "#{install_dir}/embedded/bin/2to3"
            end

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

        elsif osx?
            # Remove linux specific configs
            delete "#{install_dir}/etc/conf.d/file_handle.d"

            # remove windows specific configs
            delete "#{install_dir}/etc/conf.d/winproc.d"

            if ENV['HARDENED_RUNTIME_MAC'] == 'true'
                hardened_runtime = "-o runtime --entitlements #{entitlements_file} "
            else
                hardened_runtime = ""
            end

            if code_signing_identity
                # Codesign everything
                command "find #{install_dir} -type f | grep -E '(\\.so|\\.dylib)' | xargs codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}'"
                command "find #{install_dir}/embedded/bin -perm +111 -type f | xargs codesign #{hardened_runtime}--force --timestamp  --deep -s '#{code_signing_identity}'"
                command "find #{install_dir}/bin -perm +111 -type f | xargs codesign #{hardened_runtime}--force --timestamp  --deep -s '#{code_signing_identity}'"
                command "codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}' '#{install_dir}/Datadog Agent.app'"
            end
        end
    end
end
