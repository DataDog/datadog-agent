name "python3"

default_version "3.8.8"

if ohai["platform"] != "windows"
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
         :sha256 => "76c0763f048e4f9b861d24da76b7dd5c7a3ba7ec086f40caedeea359263276f7"

  relative_path "Python-#{version}"

  python_configure = ["./configure",
                      "--prefix=#{install_dir}/embedded",
                      "--with-ssl=#{install_dir}/embedded",
                      "--with-ensurepip=no"] # pip is installed separately by its own software def

  if mac_os_x?
    python_configure.push("--enable-ipv6",
                          "--with-universal-archs=intel",
                          "--enable-shared")
  elsif linux?
    python_configure.push("--enable-shared",
                          "--enable-ipv6")
  elsif aix?
    # something here...
  end

  python_configure.push("--with-dbmliborder=")

  build do
    ship_license "PSFL"

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
  dependency "vc_redist_14"
  #
  # note for next version after 3.8.1, remove the `-withcrt` as the filename won't
  # include that any more
  #
  if windows_arch_i386?
    dependency "vc_ucrt_redist"

    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-x86.zip",
            :sha256 => "a48b27d3d2eda3c265aaf4e06cbf26e58e669216fce48e5ec3d1c845309e5d50"
  else

    # note that startring with 3.7.3 on Windows, the zip should be created without the built-in pip
    source :url => "https://dd-agent-omnibus.s3.amazonaws.com/python-windows-#{version}-amd64.zip",
         :sha256 => "6f7c318cd0171bb8c6849797ceba31573856a3991ae59d65126bf10499be464a"

  end
  vcrt140_root = "#{Omnibus::Config.source_dir()}/vc_redist_140/expanded"
  build do
    command "XCOPY /YEHIR *.* \"#{windows_safe_path(python_3_embedded)}\""
    command "copy /y \"#{windows_safe_path(vcrt140_root)}\\*.dll\" \"#{windows_safe_path(python_3_embedded)}\""
  end
end
