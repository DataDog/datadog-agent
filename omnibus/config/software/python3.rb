name "python3"

default_version "3.12.6"

if ohai["platform"] != "windows"

  dependency "libxcrypt"
  dependency "libffi"
  dependency "ncurses"
  dependency "zlib"
  dependency "openssl3"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
  dependency "libyaml"

  source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
         :sha256 => "85a4c1be906d20e5c5a69f2466b00da769c221d6a684acfd3a514dbf5bf10a66"

  relative_path "Python-#{version}"

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

  build do
    # 2.0 is the license version here, not the python version
    license "Python-2.0"

    env = with_standard_compiler_flags(with_embedded_path)
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
  end

else
  dependency "vc_redist_14"

  # note that starting with 3.7.3 on Windows, the zip should be created without the built-in pip
  source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-amd64.zip",
         :sha256 => "045d20a659fe80041b6fd508b77f250b03330347d64f128b392b88e68897f5a0".downcase

  vcrt140_root = "#{Omnibus::Config.source_dir()}/vc_redist_140/expanded"
  build do
    # 2.0 is the license version here, not the python version
    license "Python-2.0"

    command "XCOPY /YEHIR *.* \"#{windows_safe_path(python_3_embedded)}\""

    # Install pip
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
    command "#{python} -m ensurepip"
  end
end
