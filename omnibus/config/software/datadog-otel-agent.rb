# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
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
        'PATH' => ["#{gopath.to_path}/bin", env['PATH']].join(File::PATH_SEPARATOR),
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
      if windows_target?
        msgoroot = ENV['MSGO_ROOT']
        if msgoroot.nil? || msgoroot.empty?
          raise "MSGO_ROOT not set"
        end
        if !File.exist?("#{msgoroot}\\bin\\go.exe")
          raise "msgo go.exe not found at #{msgoroot}\\bin\\go.exe"
        end
        env["GOROOT"] = msgoroot
        env["PATH"] = "#{msgoroot}\\bin;#{env['PATH']}"
        # also update the global env so that the symbol inspector use the correct go version
        ENV['GOROOT'] = msgoroot
        ENV['PATH'] = "#{msgoroot}\\bin;#{ENV['PATH']}"
      else
        msgoroot = "/usr/local/msgo"
        env["GOROOT"] = msgoroot
        env["PATH"] = "#{msgoroot}/bin:#{env['PATH']}"
        # also update the global env so that the symbol inspector use the correct go version
        ENV['GOROOT'] = msgoroot
        ENV['PATH'] = "#{msgoroot}/bin:#{ENV['PATH']}"
      end
    end

    if windows_target?
      conf_dir = "#{install_dir}/etc/datadog-agent"
    else
      conf_dir = "/etc/datadog-agent"
    end
    embedded_bin_dir = "#{install_dir}/embedded/bin"

    mkdir conf_dir
    mkdir embedded_bin_dir

    command "dda inv -- -e otel-agent.build --flavor #{flavor_arg}", :env => env, :live_stream => Omnibus.logger.live_stream(:info)

    if windows_target?
      copy 'bin/otel-agent/otel-agent.exe', embedded_bin_dir
    else
      copy 'bin/otel-agent/otel-agent', embedded_bin_dir
    end
    move 'bin/otel-agent/dist/otel-config.yaml', "#{conf_dir}/otel-config.yaml.example"

    # Check that the build tags had an actual effect:
    # the build tags added by fips mode (https://github.com/DataDog/datadog-agent/blob/7.75.1/tasks/build_tags.py#L140)
    # only have the desired effect with the microsoft go compiler
    # and are silently ignored by other compilers.
    # As a consequence the build succeeding isn't enough of a guarantee, we need to check the symbols
    # for a proof that openSSL is used
    if fips_mode?
      if linux_target?
        block do
          bin = "#{embedded_bin_dir}/otel-agent"
          symbol = "_Cfunc__mkcgo_OPENSSL"

          check_block = Proc.new { |binary, symbols|
            count = symbols.scan(symbol).count
            if count > 0
              log.info(log_key) { "Symbol '#{symbol}' found #{count} times in binary '#{binary}'." }
            else
              raise FIPSSymbolsNotFound.new("Expected to find '#{symbol}' symbol in #{binary} but did not")
            end
          }.curry

          partially_applied_check = check_block.call(bin)
          GoSymbolsInspector.new(bin, &partially_applied_check).inspect()
        end
      end
    end
end
