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

  # On Linux, wheel-bundled shared libraries (including OpenSSL for both FIPS and non-FIPS)
  # are deduplicated against #{install_dir}/embedded/lib in datadog-agent-finalize.rb once
  # all Agent embedded libraries have been installed.
  # On FIPS Windows, rebuild `cryptography` so that it dynamically links against Agent's OpenSSL.
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
