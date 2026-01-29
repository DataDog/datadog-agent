name "python3"

default_version "3.13.11"

unless windows?
  dependency "libffi"
  dependency "zlib"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
end
dependency "openssl3"

source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
       :sha256 => "03cfedbe06ce21bc44ce09245e091a77f2fee9ec9be5c52069048a181300b202"

relative_path "Python-#{version}"

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  if !windows_target?
    env = with_standard_compiler_flags(with_embedded_path)
    command_on_repo_root "bazelisk run -- @cpython//:install --destdir='#{install_dir}/embedded'"
    sh_lib = if linux_target? then "libpython3.so" else "libpython3.13.dylib" end
    command_on_repo_root "bazelisk run -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
      " #{install_dir}/embedded/lib/pkgconfig/python*.pc" \
      " #{install_dir}/embedded/lib/#{sh_lib}" \
      " #{install_dir}/embedded/lib/python3.13/lib-dynload/*.so" \
      " #{install_dir}/embedded/bin/python3*"
  elsif fips_mode?
    ###############################
    # Setup openssl dependency... #
    ###############################

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
      command "sed -i \"s/^CC=[[:space:]]*${CC}/CC=gcc/\" #{install_dir}/embedded/lib/python#{major}.#{minor}/config-#{major}.#{minor}-*-linux-gnu/Makefile", :env => env
      command "sed -i \"s/${CC}/gcc/g\" #{install_dir}/embedded/lib/python#{major}.#{minor}/_sysconfigdata__linux_*-linux-gnu.py", :env => env
    end
    if linux_target? && env["CXX"]
      command "sed -i \"s/^CXX=[[:space:]]*${CXX}/CC=g++/\" #{install_dir}/embedded/lib/python#{major}.#{minor}/config-#{major}.#{minor}-*-linux-gnu/Makefile", :env => env
      command "sed -i \"s/${CXX}/g++/g\" #{install_dir}/embedded/lib/python#{major}.#{minor}/_sysconfigdata__linux_*-linux-gnu.py", :env => env
    end
    delete "#{install_dir}/embedded/lib/python#{major}.#{minor}/test"
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python#{major}.#{minor}/distutils/command/wininst-*.exe"))
    end
  else
    fips_flag = fips_mode? ? "--//:fips_mode" : ""
    command_on_repo_root "bazelisk run #{fips_flag} -- @cpython//:install --destdir=#{python_3_embedded}"
  end
end

