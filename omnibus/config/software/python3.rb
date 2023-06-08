name "python3"

if ohai["platform"] != "windows"
  default_version "3.9.17"

  dependency "libxcrypt"
  dependency "libffi"
  dependency "ncurses"
  dependency "zlib"
  dependency ENV["OMNIBUS_OPENSSL_SOFTWARE"] || "openssl"
  dependency "pkg-config"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
  dependency "libyaml"

  source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
         :sha256 => "8ead58f669f7e19d777c3556b62fae29a81d7f06a7122ff9bc57f7dd82d7e014"

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
    delete "#{install_dir}/embedded/lib/python3.9/test"

    # There exists no configure flag to tell Python to not compile readline support :(
    major, minor, bugfix = version.split(".")
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python#{major}.#{minor}/lib-dynload/readline.*"))
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python#{major}.#{minor}/distutils/command/wininst-*.exe"))
    end
  end

else
  default_version "3.9.17-18093c4"
  dependency "vc_redist_14"

  if windows_arch_i386?
    dependency "vc_ucrt_redist"

    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-x86.zip",
            :sha256 => "07A1CD790D258AE925502E362701DED8B7362418766B89FE3CF53DB16D349A9C".downcase
  else

    # note that startring with 3.7.3 on Windows, the zip should be created without the built-in pip
    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-x64.zip",
           :sha256 => "DF4C583CF359C48AB31E83DD6E89FA08692190D91EBCEBC55BB5EDC97E04F4AA".downcase

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
