name "python3"

if ohai["platform"] != "windows"
  default_version "3.8.17"

  dependency "libxcrypt"
  dependency "libffi"
  dependency "ncurses"
  dependency "zlib"
  dependency "openssl"
  dependency "pkg-config"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
  dependency "libyaml"

  source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
         :sha256 => "def428fa6cf61b66bcde72e3d9f7d07d33b2e4226f04f9d6fce8384c055113ae"

  relative_path "Python-#{version}"

  python_configure = ["./configure",
                      "--prefix=#{install_dir}/embedded",
                      "--with-ssl=#{install_dir}/embedded",
                      "--with-ensurepip=yes"] # We upgrade pip later, in the pip3 software definition

  if mac_os_x?
    python_configure.push("--enable-ipv6",
                          "--with-universal-archs=intel",
                          "--enable-shared",
                          "--disable-static")
  elsif linux?
    python_configure.push("--enable-shared",
                          "--disable-static",
                          "--enable-ipv6")
  elsif aix?
    # something here...
  end

  python_configure.push("--with-dbmliborder=")

  build do
    # 2.0 is the license version here, not the python version
    license "Python-2.0"

    env = case ohai["platform"]
          when "aix"
            aix_env
          else
            {
              "CFLAGS" => "-I#{install_dir}/embedded/include -O2 -g -pipe",
              "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
              "PKG_CONFIG" => "#{install_dir}/embedded/bin/pkg-config",
              "PKG_CONFIG_PATH" => "#{install_dir}/embedded/lib/pkgconfig"
            }
          end
    command python_configure.join(" "), :env => env
    command "make -j #{workers}", :env => env
    command "make install", :env => env
    delete "#{install_dir}/embedded/lib/python3.8/test"

    # There exists no configure flag to tell Python to not compile readline support :(
    major, minor, bugfix = version.split(".")
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python#{major}.#{minor}/lib-dynload/readline.*"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python#{major}.#{minor}/distutils/command/wininst-*.exe"))
    end
  end

else
  default_version "3.8.17-e0a363f"
  dependency "vc_redist_14"

  if windows_arch_i386?
    dependency "vc_ucrt_redist"

    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-x86.zip",
            :sha256 => "A72C0D9CBF5235F28811A339723D1D05CE4FA95578153DEDA851119F2E099433".downcase
  else

    # note that startring with 3.7.3 on Windows, the zip should be created without the built-in pip
    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-x64.zip",
         :sha256 => "27D3B4AB04929F42119318309BD4F08CDAC6D89B3A3BE4290A27CB8CB4556212".downcase

  end
  vcrt140_root = "#{Omnibus::Config.source_dir()}/vc_redist_140/expanded"
  build do
    # 2.0 is the license version here, not the python version
    license "Python-2.0"

    command "XCOPY /YEHIR *.* \"#{windows_safe_path(python_3_embedded)}\""
    command "copy /y \"#{windows_safe_path(vcrt140_root)}\\*.dll\" \"#{windows_safe_path(python_3_embedded)}\""

    # Install pip
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
    command "#{python} -m ensurepip"
  end
end
