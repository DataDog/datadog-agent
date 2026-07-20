# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'

name 'datadog-agent-integrations-py3'

dependency 'python3'

python_version = "3.13"

whitelist_file "embedded/lib/python#{python_version}/site-packages/.libsaerospike"
whitelist_file "embedded/lib/python#{python_version}/site-packages/aerospike.libs"
whitelist_file "embedded/lib/python#{python_version}/site-packages/psycopg_binary.libs"
whitelist_file "embedded/lib/python#{python_version}/site-packages/pymqi"

site_packages_path = "#{install_dir}/embedded/lib/python#{python_version}/site-packages"
if windows_target?
  site_packages_path = "#{python_3_embedded}/Lib/site-packages"
end

build do
  # aliases for pip
  if windows_target?
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  else
    python = "#{install_dir}/embedded/bin/python3"
  end

  # Install integrations and their configs
  command "bazel run " \
          "--//packages/agent:flavor=#{ENV.fetch('AGENT_FLAVOR', 'base')} " \
          "--//:install_dir=#{install_dir} " \
          "-- //deps/agent_integrations:install --destdir=#{install_dir}",
    :live_stream => Omnibus.logger.live_stream(:info)

  # Run pip check to make sure the agent's python environment is clean, all the dependencies are compatible
  command "#{python} -B -m pip check"

  unless windows_target?
    block "Remove .exe files" do
      # setuptools come from supervisor and ddtrace
      FileUtils.rm_f(Dir.glob("#{site_packages_path}/setuptools/*.exe"))
    end
  end

  # Remove openssl copies from libraries that depend on it, and patch as necessary.
  # The OpenSSL setup with FIPS is more delicate than in the regular Agent because it makes it harder
  # to control FIPS initialization; this has surfaced as problems with `cryptography` specifically, and
  # later with `psycopg` (for the postgres integration).
  # TODO(agent-build) This is intended as a temporary kludge while we make a decision on how to handle the multiplicity
  # of openssl copies in a more general way while keeping risk low.
  if fips_mode?
    if linux_target?
      block "Patch cryptography's openssl linking" do
        # We delete the libraries shipped with the wheel and replace references to those names
        # in the binary that references it using patchelf
        cryptography_folder = "#{site_packages_path}/cryptography"
        so_to_patch = "#{cryptography_folder}/hazmat/bindings/_rust.abi3.so"
        libssl_matches = Dir.glob("#{cryptography_folder}.libs/libssl-*.so.3")
        libcrypto_matches = Dir.glob("#{cryptography_folder}.libs/libcrypto-*.so.3")
        raise "expected exactly one match for 'libssl-*.so.3' but got: #{libssl_matches}" if libssl_matches.size != 1
        raise "expected exactly one match for 'libcrypto-*.so.3' but got: #{libcrypto_matches}" if libcrypto_matches.size != 1
        libssl_match = libssl_matches.fetch(0)
        libcrypto_match = libcrypto_matches.fetch(0)
        shellout! "patchelf --replace-needed #{File.basename(libssl_match)} libssl.so.3 #{so_to_patch}"
        shellout! "patchelf --replace-needed #{File.basename(libcrypto_match)} libcrypto.so.3 #{so_to_patch}"
        shellout! "patchelf --add-rpath #{install_dir}/embedded/lib #{so_to_patch}"
        FileUtils.rm([libssl_match, libcrypto_match])
      end

      block "Patch psycopg's openssl linking" do
        # Same for psycopg
        psycopg_folder = "#{site_packages_path}/psycopg_c"
        libssl_matches = Dir.glob("#{psycopg_folder}.libs/libssl-*.so.3")
        libcrypto_matches = Dir.glob("#{psycopg_folder}.libs/libcrypto-*.so.3")
        raise "expected exactly one match for 'libssl-*.so.3' but got: #{libssl_matches}" if libssl_matches.size != 1
        raise "expected exactly one match for 'libcrypto-*.so.3' but got: #{libcrypto_matches}" if libcrypto_matches.size != 1
        libssl_match = libssl_matches.fetch(0)
        libcrypto_match = libcrypto_matches.fetch(0)

        # Files that might refer to the OpenSSL libraries and that need patching.
        # Note that if we miss any file that would need patching, the Omnibus health check will have our back
        sos_to_patch = [
          Dir.glob("#{psycopg_folder}/_psycopg.cpython-*-linux-gnu.so").fetch(0),
          Dir.glob("#{psycopg_folder}/pq.cpython-*-linux-gnu.so").fetch(0),
          Dir.glob("#{psycopg_folder}.libs/libpq-*.so*").fetch(0),
        ]
        sos_to_patch.each do |so_to_patch|
          shellout! "patchelf --replace-needed #{File.basename(libssl_match)} libssl.so.3 #{so_to_patch}"
          shellout! "patchelf --replace-needed #{File.basename(libcrypto_match)} libcrypto.so.3 #{so_to_patch}"
          shellout! "patchelf --add-rpath #{install_dir}/embedded/lib #{so_to_patch}"
        end
        FileUtils.rm([libssl_match, libcrypto_match])
      end
    elsif windows_target?
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
end
