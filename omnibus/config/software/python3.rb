name "python3"

default_version "3.12.11"

unless windows?
  dependency "libxcrypt"
  dependency "libffi"
  dependency "zlib"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
  dependency "libyaml"
end
dependency "openssl3"

source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
       :sha256 => "7b8d59af8216044d2313de8120bfc2cc00a9bd2e542f15795e1d616c51faf3d6"

relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  unless windows_target?
    env = with_standard_compiler_flags(with_embedded_path)
    python_configure_options = [
      "--without-readline",  # Disables readline support
      "--with-ensurepip=yes", # We upgrade pip later, in the pip3 software definition
      "--without-static-libpython" # We only care about the shared library
    ]

    if mac_os_x?
      python_configure_options.push("--enable-ipv6",
                            "--with-universal-archs=#{arm_target? ? "universal2" : "intel"}",
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

    ###############################
    # Setup openssl dependency... #
    ###############################

    # We must provide python with the same file hierarchy as
    # https://github.com/python/cpython-bin-deps/tree/openssl-bin-3.0/amd64
    # but with our OpenSSL build instead.

    # This is not necessarily the version we built, but the version
    # the Python build system expects.
    openssl_version = "3.0.16.2"
    python_arch = "amd64"

    mkdir "externals\\openssl-bin-#{openssl_version}\\#{python_arch}\\include"
    # Copy the import library to have them point at our own built versions, regardless of
    # their names in usual python builds
    copy "#{install_dir}\\embedded3\\lib\\libcrypto.dll.a", "externals\\openssl-bin-#{openssl_version}\\#{python_arch}\\libcrypto.lib"
    copy "#{install_dir}\\embedded3\\lib\\libssl.dll.a", "externals\\openssl-bin-#{openssl_version}\\#{python_arch}\\libssl.lib"
    # Copy the actual DLLs, be sure to keep the same name since that's what the IMPLIBs expect
    copy "#{install_dir}\\embedded3\\bin\\libssl-3-x64.dll", "externals\\openssl-bin-#{openssl_version}\\#{python_arch}\\libssl-3.dll"
    # Create empty PDBs since python's build system require those to be present
    command "touch externals\\openssl-bin-#{openssl_version}\\#{python_arch}\\libssl-3.pdb"
    copy "#{install_dir}\\embedded3\\bin\\libcrypto-3-x64.dll", "externals\\openssl-bin-#{openssl_version}\\#{python_arch}\\libcrypto-3.dll"
    command "touch externals\\openssl-bin-#{openssl_version}\\#{python_arch}\\libcrypto-3.pdb"
    # And finally the headers:
    copy "#{install_dir}\\embedded3\\include\\openssl", "externals\\openssl-bin-#{openssl_version}\\#{python_arch}\\include\\"
    # Now build python itself...

    ###############################
    # Build Python...             #
    ###############################
    # -e to enable external libraries. They won't be fetched if already
    # present, but the modules will be built nonetheless.
    command "PCbuild\\build.bat -e --pgo"
    # Install the built artifacts to their expected locations
    # --include-dev - include include/ and libs/ directories
    # --include-venv - necessary for ensurepip to work
    # --include-stable - adds python3.dll
    command "PCbuild\\#{python_arch}\\python.exe PC\\layout\\main.py --build PCbuild\\#{python_arch} --precompile --copy #{windows_safe_path(python_3_embedded)} --include-dev --include-venv --include-stable -vv"

    ###############################
    # Install build artifacts...  #
    ###############################
    # We copied the OpenSSL libraries with the name python expects to keep the build happy
    # but at runtime, it will attempt to load the DLLs pointed at by the .dll.a generated by
    # the OpenSSL build, so we need to copy those files to the install directory.
    # The ones we copied for the build are now irrelevant
    openssl_arch = "x64"
    copy "#{install_dir}\\embedded3\\bin\\libcrypto-3-#{openssl_arch}.dll", "#{windows_safe_path(python_3_embedded)}\\DLLs"
    copy "#{install_dir}\\embedded3\\bin\\libssl-3-#{openssl_arch}.dll", "#{windows_safe_path(python_3_embedded)}\\DLLs"
    # We can also remove the DLLs that were put there by the python build since they won't be loaded anyway
    delete "#{windows_safe_path(python_3_embedded)}\\DLLs\\libcrypto-3.dll"
    delete "#{windows_safe_path(python_3_embedded)}\\DLLs\\libssl-3.dll"
    # Generate libpython3XY.a for MinGW tools
    # https://docs.python.org/3/whatsnew/3.8.html
    major, minor, _ = version.split(".")
    command "gendef #{windows_safe_path(python_3_embedded)}\\python#{major}#{minor}.dll"
    command "dlltool --dllname python#{major}#{minor}.dll --def python#{major}#{minor}.def --output-lib #{windows_safe_path(python_3_embedded)}\\libs\\libpython#{major}#{minor}.a"

    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
    command "#{python} -m ensurepip"
  end
end

