# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'json'

name 'datadog-agent-integrations-py3'

license "BSD-3-Clause"
license_file "./LICENSE"

dependency 'datadog-agent-integrations-py3-dependencies'

python_version = "3.12"

relative_path 'integrations-core'
whitelist_file "embedded/lib/python#{python_version}/site-packages/.libsaerospike"
whitelist_file "embedded/lib/python#{python_version}/site-packages/aerospike.libs"
whitelist_file "embedded/lib/python#{python_version}/site-packages/psycopg_binary.libs"
whitelist_file "embedded/lib/python#{python_version}/site-packages/pymqi"

source git: 'https://github.com/DataDog/integrations-core.git'

integrations_core_version = ENV['INTEGRATIONS_CORE_VERSION']
if integrations_core_version.nil? || integrations_core_version.empty?
  integrations_core_version = 'master'
end
default_version integrations_core_version

# folder names containing integrations from -core that won't be packaged with the Agent
excluded_folders = [
  'datadog_checks_base',           # namespacing package for wheels (NOT AN INTEGRATION)
  'datadog_checks_dev',            # Development package, (NOT AN INTEGRATION)
  'datadog_checks_tests_helper',   # Testing and Development package, (NOT AN INTEGRATION)
  'docker_daemon',                 # Agent v5 only
]

if osx_target?
  # Temporarily exclude Aerospike until builder supports new dependency
  excluded_folders.push('aerospike')
  excluded_folders.push('teradata')
end

if arm_target?
  # This doesn't build on ARM
  excluded_folders.push('ibm_ace')
  excluded_folders.push('ibm_mq')
end

final_constraints_file = 'final_constraints-py3.txt'
agent_requirements_file = 'agent_requirements-py3.txt'
filtered_agent_requirements_in = 'agent_requirements-py3.in'
agent_requirements_in = 'agent_requirements.in'

site_packages_path = "#{install_dir}/embedded/lib/python#{python_version}/site-packages"
if windows_target?
  site_packages_path = "#{python_3_embedded}/Lib/site-packages"
end

build do
  # The dir for confs
  if osx_target?
    conf_dir = "#{install_dir}/etc/conf.d"
  else
    conf_dir = "#{install_dir}/etc/datadog-agent/conf.d"
  end
  mkdir conf_dir

  # aliases for pip
  if windows_target?
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  else
    python = "#{install_dir}/embedded/bin/python3"
  end

  # If a python_mirror is set, it is set in a pip config file so that we do not leak the token in the CI output
  pip_config_file = ENV['PIP_CONFIG_FILE']
  pre_build_env = {
    "PIP_CONFIG_FILE" => "#{pip_config_file}"
  }

  # Install dependencies
  lockfile_name = case
    when linux_target?
      arm_target? ? "linux-aarch64" : "linux-x86_64"
    when osx_target?
      "macos-x86_64"
    when windows_target?
      "windows-x86_64"
  end + "_#{python_version}.txt"
  lockfile = windows_safe_path(project_dir, ".deps", "resolved", lockfile_name)
  command "#{python} -m pip install --require-hashes --only-binary=:all: --no-deps -r #{lockfile}"

  # Prepare build env for integrations
  wheel_build_dir = windows_safe_path(project_dir, ".wheels")
  build_deps_dir = windows_safe_path(project_dir, ".build_deps")
  # We download build dependencies to make them available without an index when installing integrations
  command "#{python} -m pip download --dest #{build_deps_dir} hatchling==0.25.1", :env => pre_build_env
  command "#{python} -m pip download --dest #{build_deps_dir} setuptools==75.1.0", :env => pre_build_env # Version from ./setuptools3.rb
  build_env = {
    "PIP_FIND_LINKS" => build_deps_dir,
    "PIP_CONFIG_FILE" => pip_config_file,
  }

  # Install base and downloader packages
  cwd_base = windows_safe_path(project_dir, "datadog_checks_base")
  cwd_downloader = windows_safe_path(project_dir, "datadog_checks_downloader")
  command "#{python} -m pip wheel . --no-deps --no-index --wheel-dir=#{wheel_build_dir}", :env => build_env, :cwd => cwd_base
  command "#{python} -m pip install datadog_checks_base --no-deps --no-index --find-links=#{wheel_build_dir}"
  command "#{python} -m pip wheel . --no-deps --no-index --wheel-dir=#{wheel_build_dir}", :env => build_env, :cwd => cwd_downloader
  command "#{python} -m pip install datadog_checks_downloader --no-deps --no-index --find-links=#{wheel_build_dir}"

  #
  # Install Core integrations
  #

  # Create a constraint file after installing all the core dependencies and before any integration
  # This is then used as a constraint file by the integration command to avoid messing with the agent's python environment
  command "#{python} -m pip freeze > #{install_dir}/#{final_constraints_file}"

  if windows_target?
    cached_wheels_dir = "#{windows_safe_path(wheel_build_dir)}\\.cached"
  else
    cached_wheels_dir = "#{wheel_build_dir}/.cached"
  end

  block "Install integrations" do
    tasks_dir_in = windows_safe_path(Dir.pwd)
    # Collect integrations to install
    checks_to_install = (
      shellout! "dda inv -- agent.collect-integrations #{project_dir} 3 #{os} #{excluded_folders.join(',')}",
                :cwd => tasks_dir_in
    ).stdout.split()
    # Retrieving integrations from cache
    cache_bucket = ENV.fetch('INTEGRATION_WHEELS_CACHE_BUCKET', '')
    cache_branch = (shellout! "dda inv -- release.get-release-json-value base_branch --no-worktree", cwd: File.expand_path('..', tasks_dir_in)).stdout.strip
    if cache_bucket != ''
      mkdir cached_wheels_dir
      shellout! "dda inv -- -e agent.get-integrations-from-cache " \
                "--python 3 --bucket #{cache_bucket} " \
                "--branch #{cache_branch || 'main'} " \
                "--integrations-dir #{windows_safe_path(project_dir)} " \
                "--target-dir #{cached_wheels_dir} " \
                "--integrations #{checks_to_install.join(',')}",
                :cwd => tasks_dir_in

      # install all wheels from cache in one pip invocation to speed things up
      if windows_target?
        shellout! "#{python} -m pip install --no-deps --no-index " \
                  " --find-links #{windows_safe_path(cached_wheels_dir)} -r #{windows_safe_path(cached_wheels_dir)}\\found.txt"
      else
        shellout! "#{python} -m pip install --no-deps --no-index " \
                  "--find-links #{cached_wheels_dir} -r #{cached_wheels_dir}/found.txt"
      end
    end

    # get list of integration wheels already installed from cache
    installed_list = Array.new
    if cache_bucket != ''
      installed_out = `#{python} -m pip list --format json`
      if $?.exitstatus == 0
        installed = JSON.parse(installed_out)
        installed.each do |package|
          package.each do |key, value|
            if key == "name" && value.start_with?("datadog-")
              installed_list.push(value["datadog-".length..-1])
            end
          end
        end
      else
        raise "Failed to list pip installed packages"
      end
    end

    checks_to_install.each do |check|
      # pip < 21.2 replace underscores by dashes in package names per https://pip.pypa.io/en/stable/news/#v21-2
      # whether or not this might switch back in the future is not guaranteed, so we check for both name
      # with dashes and underscores
      if !(installed_list.include?(check) || installed_list.include?(check.gsub('_', '-')))
        if windows_target?
          shellout! "#{python} -m pip wheel . --no-deps --no-index --wheel-dir=#{wheel_build_dir}", :env => build_env, :cwd => "#{windows_safe_path(project_dir)}\\#{check}"
        else
          shellout! "#{python} -m pip wheel . --no-deps --no-index --wheel-dir=#{wheel_build_dir}", :env => build_env, :cwd => "#{project_dir}/#{check}"
        end
        shellout! "#{python} -m pip install datadog-#{check} --no-deps --no-index --find-links=#{wheel_build_dir}"
        if cache_bucket != '' && ENV.fetch('INTEGRATION_WHEELS_SKIP_CACHE_UPLOAD', '') == '' && cache_branch != nil
          shellout! "dda inv -- -e agent.upload-integration-to-cache " \
                    "--python 3 --bucket #{cache_bucket} " \
                    "--branch #{cache_branch} " \
                    "--integrations-dir #{windows_safe_path(project_dir)} " \
                    "--build-dir #{wheel_build_dir} " \
                    "--integration #{check}",
                    :cwd => tasks_dir_in
        end
      end

      check_dir = File.join(project_dir, check)
      check_conf_dir = "#{conf_dir}/#{check}.d"

      # For each conf file, if it already exists, that means the `datadog-agent` software def
      # wrote it first. In that case, since the agent's confs take precedence, skip the conf
      conf_files = ["conf.yaml.example", "conf.yaml.default", "metrics.yaml", "auto_conf.yaml"]
      conf_files.each do |filename|
        src = windows_safe_path(check_dir, "datadog_checks", check, "data", filename)
        dest = check_conf_dir
        if File.exist?(src) and !File.exist?(windows_safe_path(dest, filename))
          FileUtils.mkdir_p(dest)
          FileUtils.cp_r(src, dest)
        end

        # Drop the example files from the installed packages since they are copied in /etc/datadog-agent/conf.d and not used here
        delete "#{site_packages_path}/datadog_checks/#{check}/data/#{filename}"
      end

      # Copy SNMP profiles
      profile_folders = ['profiles', 'default_profiles']
      profile_folders.each do |profile_folder|
        folder_path = "#{check_dir}/datadog_checks/#{check}/data/#{profile_folder}"
        if File.exist? folder_path
          FileUtils.cp_r folder_path, "#{check_conf_dir}/"
        end
      end
    end
  end

  # Run pip check to make sure the agent's python environment is clean, all the dependencies are compatible
  command "#{python} -m pip check"

  # Removing tests that don't need to be shipped in the embedded folder
  test_folders = [
    '../idlelib/idle_test',
    'bs4/tests',
    'Cryptodome/SelfTest',
    'gssapi/tests',
    'keystoneauth1/tests',
    'lazy_loader/tests',
    'openstack/tests',
    'os_service_types/tests',
    'pbr/tests',
    'pkg_resources/tests',
    'pip/_vendor/colorama/tests',
    'psutil/tests',
    'requests_unixsocket/tests',
    'securesystemslib/_vendor/ed25519/test_data',
    'setuptools/_distutils/compilers/C/tests',
    'setuptools/_distutils/tests',
    'setuptools/tests',
    'simplejson/tests',
    'stevedore/tests',
    'supervisor/tests',
    'test', # cm-client
    'vertica_python/tests',
    'websocket/tests',
    'win32com/test',
  ]
  test_folders.each do |test_folder|
    delete "#{site_packages_path}/#{test_folder}/"
  end

  unless windows_target?
    block "Remove .exe files" do
      # setuptools come from supervisor and ddtrace
      FileUtils.rm_f(Dir.glob("#{site_packages_path}/setuptools/*.exe"))
    end
  end

  # Remove openssl copies from cryptography, and patch as necessary.
  # The OpenSSL setup with FIPS is more delicate than in the regular Agent because it makes it harder
  # to control FIPS initialization; this has surfaced as problems with `cryptography` specifically, because
  # it's the only dependency that links to openssl needed to enable FIPS on the subset of integrations
  # that we target.
  # This is intended as a temporary kludge while we make a decision on how to handle the multiplicity
  # of openssl copies in a more general way while keeping risk low.
  if fips_mode?
    if linux_target?
      block "Patch cryptography's openssl linking" do
        # We delete the libraries shipped with the wheel and replace references to those names
        # in the binary that references it using patchelf
        cryptography_folder = "#{site_packages_path}/cryptography"
        so_to_patch = "#{cryptography_folder}/hazmat/bindings/_rust.abi3.so"
        libssl_match = Dir.glob("#{cryptography_folder}.libs/libssl-*.so.3")[0]
        libcrypto_match = Dir.glob("#{cryptography_folder}.libs/libcrypto-*.so.3")[0]
        shellout! "patchelf --replace-needed #{File.basename(libssl_match)} libssl.so.3 #{so_to_patch}"
        shellout! "patchelf --replace-needed #{File.basename(libcrypto_match)} libcrypto.so.3 #{so_to_patch}"
        shellout! "patchelf --add-rpath #{install_dir}/embedded/lib #{so_to_patch}"
        FileUtils.rm([libssl_match, libcrypto_match])
      end
    elsif windows_target?
      # Build the cryptography library in this case so that it gets linked to Agent's OpenSSL
      lib_folder = File.join(install_dir, "embedded3", "lib")
      dll_folder = File.join(install_dir, "embedded3", "DLLS")
      include_folder = File.join(install_dir, "embedded3", "include")

      # We first need create links to some files around such that cryptography finds .lib files
      link File.join(lib_folder, "libssl.dll.a"),
           File.join(dll_folder, "libssl-3-x64.lib")
      link File.join(lib_folder, "libcrypto.dll.a"),
           File.join(dll_folder, "libcrypto-3-x64.lib")

      block "Build cryptopgraphy library against Agent's OpenSSL" do
        cryptography_requirement = (shellout! "#{python} -m pip list --format=freeze").stdout[/cryptography==.*?$/]

        shellout! "#{python} -m pip install --force-reinstall --no-deps --no-binary cryptography #{cryptography_requirement}",
                env: {
                  "OPENSSL_LIB_DIR" => dll_folder,
                  "OPENSSL_INCLUDE_DIR" => include_folder,
                  "OPENSSL_LIBS" => "libssl-3-x64:libcrypto-3-x64",
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

  # These are files containing Python type annotations which aren't used at runtime
  libraries = [
    'krb5',
    'Cryptodome',
    'ddtrace',
    'pyVmomi',
    'gssapi',
  ]
  block "Remove type annotations files" do
    libraries.each do |library|
      FileUtils.rm_f(Dir.glob("#{site_packages_path}/#{library}/**/*.pyi"))
      FileUtils.rm_f(Dir.glob("#{site_packages_path}/#{library}/**/py.typed"))
    end
  end

  # Ship `requirements-agent-release.txt` file containing the versions of every check shipped with the agent
  # Used by the `datadog-agent integration` command to prevent downgrading a check to a version
  # older than the one shipped in the agent
  copy "#{project_dir}/requirements-agent-release.txt", "#{install_dir}/"
end
