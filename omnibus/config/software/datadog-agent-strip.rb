# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

# This software definition doesn"t build anything, it"s the place where we create
# files outside the omnibus installation directory, so that we can add them to
# the package manifest using `extra_package_file` in the project definition.
require './lib/ostools.rb'

name "datadog-agent-strip"
description "strip symbols from the build"
default_version "1.0.0"
skip_transitive_dependency_licensing true

build do
    block do
        # only stripping linux for now
        if linux?
            # Strip symbols
            # strip_symbols(install_dir, "#{install_dir}/symbols")
            # debug_path "**/symbols"  # this is a little dangerous actually...
            # delete "#{install_dir}/symbols"
            #
            path = install_dir
            symboldir = "#{install_dir}/symbols"
            read_output_lines("find #{path}/ -type f -exec file {} \; | grep 'ELF' | cut -f1 -d:") do |elf|
	            debugfile = "#{elf}.debug"
                elfdir = File.dirname(debugfile)
                FileUtils.mkdir_p "#{symboldir}/#{elfdir}" unless Dir.exist? "#{symboldir}/#{elfdir}"

                log.debug(log_key) { "stripping ${elf}, putting debug info into ${debugfile}" }
	            shellout("objcopy --only-keep-debug #{elf} #{symboldir}/#{debugfile}")
	            shellout("strip --strip-debug --strip-unneeded #{elf}")
	            shellout("objcopy --add-gnu-debuglink=#{symboldir}/#{debugfile} #{elf}")
                shellout("chmod -x #{symboldir}/#{debugfile}")
            end
        end
    end
end
