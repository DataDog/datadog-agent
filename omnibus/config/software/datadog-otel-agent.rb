# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require './lib/fips.rb'
require './lib/project_helpers.rb'
require 'pathname'

name 'datadog-otel-agent'

source path: '..',
       options: {
         exclude: [
           "**/.cache/**/*",
           "**/testdata/**/*",
         ],
       }
relative_path 'src/github.com/DataDog/datadog-agent'

always_build true

build do
    license :project_license

    # set GOPATH on the omnibus source dir for this software
    gopath = Pathname.new(project_dir) + '../../../..'
    flavor_arg = ENV['AGENT_FLAVOR']

    # include embedded path (mostly for `pkg-config` binary)
    #
    # with_embedded_path prepends the embedded path to the PATH from the global environment
    # in particular it ignores the PATH from the environment given as argument
    # so we need to call it before setting the PATH
    env = with_embedded_path()
    env = {
        'GOPATH' => gopath.to_path,
        'PATH' => [gopath / 'bin', env['PATH']].join(File::PATH_SEPARATOR),
        "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
        "CGO_CFLAGS" => "-I. -I#{install_dir}/embedded/include",
        "CGO_LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib"
    }

    unless ENV["OMNIBUS_GOMODCACHE"].nil? || ENV["OMNIBUS_GOMODCACHE"].empty?
        gomodcache = Pathname.new(ENV["OMNIBUS_GOMODCACHE"])
        env["GOMODCACHE"] = gomodcache.to_path
    end

    env = with_standard_compiler_flags(env)

    if fips_mode?
      add_msgo_to_env(env)
    end

    if windows_target?
      conf_dir = File.join(install_dir, 'etc', 'datadog-agent')
      binary_name = 'otel-agent.exe'
    else
      conf_dir = "/etc/datadog-agent"
      binary_name = 'otel-agent'
    end
    embedded_bin_dir = File.join(install_dir, 'embedded', 'bin')

    mkdir conf_dir
    mkdir embedded_bin_dir

    command "dda inv -- -e otel-agent.build --flavor #{flavor_arg}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)

    copy File.join('bin', 'otel-agent', binary_name), embedded_bin_dir
    move 'bin/otel-agent/dist/otel-config.yaml', File.join(conf_dir, 'otel-config.yaml.example')

    if fips_mode?
      block do
        fips_check_binary_for_expected_symbol(File.join(embedded_bin_dir, binary_name))
      end
    end
end
