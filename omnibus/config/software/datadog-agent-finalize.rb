# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

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
            conf_dir_root = "#{Omnibus::Config.source_dir()}/etc/datadog-agent"
            conf_dir = "#{conf_dir_root}/extra_package_files/EXAMPLECONFSLOCATION"
            mkdir conf_dir
            move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", conf_dir_root, :force=>true
            move "#{install_dir}/etc/datadog-agent/conf.d/*", conf_dir, :force=>true
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
            move "#{install_dir}/scripts/datadog-agent.conf", "/etc/init"
            move "#{install_dir}/scripts/datadog-agent-trace.conf", "/etc/init"
            move "#{install_dir}/scripts/datadog-agent-process.conf", "/etc/init"
            move "#{install_dir}/scripts/datadog-agent-sysprobe.conf", "/etc/init"
            systemd_directory = "/usr/lib/systemd/system"
            if debian?
                # debian recommends using a different directory for systemd unit files
                systemd_directory = "/lib/systemd/system"

                # sysvinit support for debian only for now
                mkdir "/etc/init.d"
                move "#{install_dir}/scripts/datadog-agent", "/etc/init.d"
                move "#{install_dir}/scripts/datadog-agent-trace", "/etc/init.d"
                move "#{install_dir}/scripts/datadog-agent-process", "/etc/init.d"
                move "#{install_dir}/scripts/datadog-agent-sysprobe", "/etc/init.d"
            end
            mkdir systemd_directory
            move "#{install_dir}/scripts/datadog-agent.service", systemd_directory
            move "#{install_dir}/scripts/datadog-agent-trace.service", systemd_directory
            move "#{install_dir}/scripts/datadog-agent-process.service", systemd_directory
            move "#{install_dir}/scripts/datadog-agent-sysprobe.service", systemd_directory

            # Move configuration files
            mkdir "/etc/datadog-agent"
            move "#{install_dir}/bin/agent/dd-agent", "/usr/bin/dd-agent"
            move "#{install_dir}/etc/datadog-agent/datadog.yaml.example", "/etc/datadog-agent"
            move "#{install_dir}/etc/datadog-agent/system-probe.yaml.example", "/etc/datadog-agent"
            move "#{install_dir}/etc/datadog-agent/conf.d", "/etc/datadog-agent", :force=>true

            # Create empty directories so that they're owned by the package
            # (also requires `extra_package_file` directive in project def)
            mkdir "/etc/datadog-agent/checks.d"
            mkdir "/var/log/datadog"

            # remove unused configs
            delete "/etc/datadog-agent/conf.d/apm.yaml.default"
            delete "/etc/datadog-agent/conf.d/process_agent.yaml.default"

            # remove windows specific configs
            delete "/etc/datadog-agent/conf.d/winproc.d"

            # cleanup clutter
            delete "#{install_dir}/etc"

            # The prerm script of the package should use this list to remove the pyc/pyo files
            command "echo '# DO NOT REMOVE/MODIFY - used by package removal tasks' > #{install_dir}/embedded/.py_compiled_files.txt"
            command "find #{install_dir}/embedded '(' -name '*.pyc' -o -name '*.pyo' ')' -type f -delete -print >> #{install_dir}/embedded/.py_compiled_files.txt"

            # removing the doc from the embedded folder to reduce package size by ~3MB
            delete "#{install_dir}/embedded/share/doc"

            # removing the terminfo db from the embedded folder to reduce package size by ~7MB
            delete "#{install_dir}/embedded/share/terminfo"
            # removing the symlink too
            delete "#{install_dir}/embedded/lib/terminfo"

            # removing useless folder
            delete "#{install_dir}/embedded/share/aclocal"
            delete "#{install_dir}/embedded/share/examples"

            # Setup script aliases, e.g. `/opt/datadog-agent/embedded/bin/pip` will default to `pip2`
            if with_python_runtime? "2"
                delete "#{install_dir}/embedded/bin/pip"
                link "#{install_dir}/embedded/bin/pip2", "#{install_dir}/embedded/bin/pip"

                delete "#{install_dir}/embedded/bin/2to3"
                link "#{install_dir}/embedded/bin/2to3-2.7", "#{install_dir}/embedded/bin/2to3"
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

            # Nothing to move on osx, the confs already live in /opt/datadog-agent/etc/
        end
    end
end
