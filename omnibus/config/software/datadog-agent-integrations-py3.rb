# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'

name 'datadog-agent-integrations-py3'

python_version = "3.13"

whitelist_file "embedded/lib/python#{python_version}/site-packages/.libsaerospike"
whitelist_file "embedded/lib/python#{python_version}/site-packages/aerospike.libs"
whitelist_file "embedded/lib/python#{python_version}/site-packages/psycopg_binary.libs"
whitelist_file "embedded/lib/python#{python_version}/site-packages/pymqi"

if osx_target?
  # On macos, the krb5 package's extensions link against the Heimdal "private framework",
  # which has no stability guarantees, but that's how this library works.
  whitelist_file "embedded/lib/python#{python_version}/site-packages/krb5"
  # Currently we use the prebuilt wheel for pyodbc on macos, which contains a reference to
  # /opt/homebrew/opt/unixodbc/lib/libodbc.2.dylib. This whitelisting reflects the current state;
  # this can probably be fixed on integrations-core so that we build this dependency like we do on linux.
  whitelist_file "embedded/lib/python#{python_version}/site-packages/pyodbc"
end

site_packages_path = "#{install_dir}/embedded/lib/python#{python_version}/site-packages"
if windows_target?
  site_packages_path = "#{python_3_embedded}/Lib/site-packages"
end

build do
  command "bazel run #{omnibazel_flags} -- @cpython//:install --destdir=#{install_dir}",
    :live_stream => Omnibus.logger.live_stream(:info)

  # aliases for pip
  if windows_target?
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  else
    python = "#{install_dir}/embedded/bin/python3"
  end

  # Install integrations and their configs
  command "bazel run #{omnibazel_flags} -- //deps/agent_integrations:install --destdir=#{install_dir}",
    :live_stream => Omnibus.logger.live_stream(:info)

  # Run pip check to make sure the agent's python environment is clean, all the dependencies are compatible
  command "#{python} -B -m pip check"

  # Integration dependency wheels from integrations-core bundle their own copy of OpenSSL: auditwheel grafts it under a
  # hashed name such as `<package>.libs/libssl-5fe3ef2a.so.3`. On Linux, point every library that loads
  # one of these copies at the Agent's OpenSSL in `embedded/lib` instead, and delete the copies. A single OpenSSL per
  # process is required for FIPS (a second copy can't load the FIPS provider the Agent's copy has already loaded, so
  # its TLS has no ciphers), and it avoids shipping a duplicate OpenSSL.
  if linux_target?
    block "Use the Agent's OpenSSL in integration dependencies" do
      agent_lib_dir = "#{install_dir}/embedded/lib"
      bundled_openssl = Dir.glob("#{site_packages_path}/**/lib{ssl,crypto}-*.so.3", File::FNM_DOTMATCH)
      agent_soname = bundled_openssl.to_h do |path|
        [File.basename(path), File.basename(path).sub(/-\h+(?=\.so\.3\z)/, "")]
      end

      # Only ELF files mentioning libssl or libcrypto can depend on them; this avoids running patchelf on all of them
      candidates = Dir.glob("#{site_packages_path}/**/*", File::FNM_DOTMATCH).select do |path|
        next false if !File.file?(path) || File.symlink?(path) || bundled_openssl.include?(path)

        content = File.binread(path)
        content.start_with?("\x7FELF".b) && (content.include?("libssl") || content.include?("libcrypto"))
      end

      openssl_users = candidates.select do |so|
        needed = shellout!("patchelf --print-needed #{so}").stdout.split
        bundled = needed.select { |lib| agent_soname.key?(lib) }
        unless bundled.empty?
          shellout! "patchelf #{bundled.map { |lib| "--replace-needed #{lib} #{agent_soname[lib]}" }.join(" ")} #{so}"
          shellout! "patchelf --add-rpath #{agent_lib_dir} #{so}"
        end
        needed.any? { |lib| lib.match?(/\Alib(ssl|crypto)[-.]/) }
      end
      FileUtils.rm(bundled_openssl)

      # Check against the Agent's actual OpenSSL that every library loading OpenSSL, directly or through its
      # dependencies, now gets the Agent's copy and finds every symbol version it needs. A wheel built against a newer
      # OpenSSL than the Agent ships fails here instead of at runtime.
      openssl_users.each do |so|
        ldd = shellout("ldd #{so}")
        output = ldd.stdout + ldd.stderr
        resolved = output.scan(/^\s*(lib(?:ssl|crypto)\S*) => (\S+)/)
        missing_versions = output.lines.grep(/lib(?:ssl|crypto)[^:]*: version .* not found/)
        if resolved.empty? || !missing_versions.empty? ||
           resolved.any? { |_, path| File.dirname(File.expand_path(path)) != agent_lib_dir }
          raise "expected #{so} to load OpenSSL from #{agent_lib_dir}, ldd reports:\n#{output}"
        end
      end

      # In FIPS builds these must use the Agent's OpenSSL, so they can't come with their own, even statically linked
      if fips_mode?
        fips_critical = [
          "#{site_packages_path}/cryptography/hazmat/bindings/_rust.abi3.so",
          *Dir.glob("#{site_packages_path}/psycopg_c.libs/libpq-*.so*"),
        ]
        missing = fips_critical - openssl_users
        raise "expected #{missing} to load OpenSSL from #{agent_lib_dir}" unless missing.empty?
      end
    end
  end

  if fips_mode? && windows_target?
    # Build the cryptography library in this case so that it gets linked to Agent's OpenSSL
    lib_folder = File.join(install_dir, "embedded3", "lib")
    include_folder = File.join(install_dir, "embedded3", "include")

    block "Build cryptography library against Agent's OpenSSL" do
      cryptography_requirement = (shellout! "#{python} -B -m pip list --format=freeze").stdout[/cryptography==.*?$/]

      shellout! "#{python} -B -m pip install --no-compile --force-reinstall --no-deps --no-binary cryptography #{cryptography_requirement}",
              env: {
                "OPENSSL_LIB_DIR" => lib_folder,
                "OPENSSL_INCLUDE_DIR" => include_folder,
              }
    end
    # Python extensions on windows require this to find their DLL dependencies,
    # we abuse the `.pth` loading system to inject it
    block "Inject dll path for Python extensions" do
      File.open(File.join(install_dir, "embedded3", "lib", "site-packages", "add-dll-directory.pth"), "w") do |f|
        f.puts 'import os; os.add_dll_directory(os.path.abspath(os.path.join(__file__, "..", "..", "DLLS")))'
      end
    end
  end
end
