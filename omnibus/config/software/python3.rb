name "python3"

default_version "3.12.6"

unless windows?
  dependency "libxcrypt"
  dependency "libffi"
  dependency "ncurses"
  dependency "zlib"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
  dependency "libyaml"
end
dependency "openssl3"

source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
       :sha256 => "85a4c1be906d20e5c5a69f2466b00da769c221d6a684acfd3a514dbf5bf10a66"

relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  unless windows_target?
    env = with_standard_compiler_flags(with_embedded_path)
    python_configure_options = [
      "--without-readline",  # Disables readline support
      "--with-ensurepip=yes" # We upgrade pip later, in the pip3 software definition
    ]

    if mac_os_x?
      python_configure_options.push("--enable-ipv6",
                            "--with-universal-archs=intel",
                            "--enable-shared")
    elsif linux_target?
      python_configure_options.push("--enable-shared",
                            "--enable-ipv6")
    elsif aix?
      # something here...
    end

    python_configure_options.push("--with-dbmliborder=")

    # Force different defaults for the "optimization settings"
    # This removes the debug symbol generation and doesn't enable all warnings
    env["OPT"] = "-DNDEBUG -fwrapv"
    configure(*python_configure_options, :env => env)
    command "make -j #{workers}", :env => env
    command "make install", :env => env

    # There exists no configure flag to tell Python to not compile readline support :(
    major, minor, bugfix = version.split(".")

    # Don't forward CC and CXX to python extensions Makefile, it's quite unlikely that any non default
    # compiler we use would end up being available in the system/docker image used by customers
    if linux_target? && env["CC"]
      command "sed -i \"s/^CC=[[:space:]]*${CC}/CC=gcc/\" #{install_dir}/embedded/lib/python#{major}.#{minor}/config-3.12-*-linux-gnu/Makefile", :env => env
      command "sed -i \"s/${CC}/gcc/g\" #{install_dir}/embedded/lib/python#{major}.#{minor}/_sysconfigdata__linux_*-linux-gnu.py", :env => env
    end
    if linux_target? && env["CXX"]
      command "sed -i \"s/^CXX=[[:space:]]*${CXX}/CC=g++/\" #{install_dir}/embedded/lib/python#{major}.#{minor}/config-3.12-*-linux-gnu/Makefile", :env => env
      command "sed -i \"s/${CXX}/g++/g\" #{install_dir}/embedded/lib/python#{major}.#{minor}/_sysconfigdata__linux_*-linux-gnu.py", :env => env
    end
    delete "#{install_dir}/embedded/lib/python#{major}.#{minor}/test"
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python#{major}.#{minor}/distutils/command/wininst-*.exe"))
    end
  else
    dependency "vc_redist_14"

    vcrt140_root = "#{Omnibus::Config.source_dir()}/vc_redist_140/expanded"
    ###############################
    # Setup openssl dependency... #
    ###############################

    # This is not necessarily the version we built, but the version
    # the Python build system expects.
    openssl_version = "3.0.15"
    python_arch = "amd64"

    mkdir "externals/openssl-bin-#{openssl_version}/#{python_arch}/include"
    # Copy the import library to have them point at our own built versions, regardless of
    # their names in usual python builds
    copy "#{install_dir}/embedded3/lib/libcrypto.dll.a", "externals/openssl-bin-#{openssl_version}/#{python_arch}/libcrypto.lib"
    copy "#{install_dir}/embedded3/lib/libssl.dll.a", "externals/openssl-bin-#{openssl_version}/#{python_arch}/libssl.lib"
    copy "#{install_dir}/embedded3/lib/libssl.dll.a", "externals/openssl-bin-#{openssl_version}/#{python_arch}/libssl.lib"
    # Copy the actual DLLs, be sure to keep the same name since that's what the IMPLIBs expect
    copy "#{install_dir}/embedded3/bin/libssl-3-x64.dll", "externals/openssl-bin-#{openssl_version}/#{python_arch}/libssl-3.dll"
    command "touch externals/openssl-bin-#{openssl_version}/#{python_arch}/libssl-3.pdb"
    copy "#{install_dir}/embedded3/bin/libcrypto-3-x64.dll", "externals/openssl-bin-#{openssl_version}/#{python_arch}/libcrypto-3.dll"
    command "touch externals/openssl-bin-#{openssl_version}/#{python_arch}/libcrypto-3.pdb"
    # The applink "header"
    copy "#{install_dir}/embedded3/include/openssl/applink.c", "externals/openssl-bin-#{openssl_version}/#{python_arch}/include/"
    # And finally the headers:
    copy "#{install_dir}/embedded3/include/openssl", "externals/openssl-bin-#{openssl_version}/#{python_arch}/include/"
    # Now build python itself...

    ###############################
    # Build Python...             #
    ###############################
    # -e to enable external libraries. They won't be fetched if already
    # present, but the modules will be built nonetheless.
    command "PCbuild\\build.bat -e --pgo"
    command "dir PCbuild/#{python_arch}/"
    # Install the build artifact to their expected locations
    copy "PCbuild/#{python_arch}/*.exe", "#{windows_safe_path(python_3_embedded)}/"
    copy "PCbuild/#{python_arch}/*.dll", "#{windows_safe_path(python_3_embedded)}/"
    mkdir "#{windows_safe_path(python_3_embedded)}/DLLs"
    copy "PCbuild/#{python_arch}/*.pyd", "#{windows_safe_path(python_3_embedded)}/DLLs/"
    mkdir "#{windows_safe_path(python_3_embedded)}/libs"
    copy "PCbuild/#{python_arch}/*.lib", "#{windows_safe_path(python_3_embedded)}/libs"
    copy "Lib", "#{windows_safe_path(python_3_embedded)}/"

    ###############################
    # Install build artifacts...  #
    ###############################
    # We copied the OpenSSL libraries with the name python expects to keep the build happy
    # but at runtime, it will attempt to load the DLLs pointed at by the .dll.a generated by
    # the OpenSSL build, so we need to copy those files to the install directory.
    # The ones we copied for the build are now irrelevant
    openssl_arch = "x64"
    copy "#{install_dir}/embedded3/bin/libcrypto-3-#{openssl_arch}.dll", "#{windows_safe_path(python_3_embedded)}/DLLs"
    copy "#{install_dir}/embedded3/bin/libssl-3-#{openssl_arch}.dll", "#{windows_safe_path(python_3_embedded)}/DLLs"
    # We can also remove the DLLs that were put there by the python build since they won't be loaded anyway
    delete "#{windows_safe_path(python_3_embedded)}/libcrypto-3.dll"
    delete "#{windows_safe_path(python_3_embedded)}/libssl-3.dll"

    copy "Include", "#{windows_safe_path(python_3_embedded)}\\include"
    copy "PC/pyconfig.h", "#{windows_safe_path(python_3_embedded)}\\include\\"

    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
    command "#{python} -m ensurepip"
  end
end

