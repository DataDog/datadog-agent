name "python3"

default_version "3.8.1"

if ohai["platform"] != "windows"
  dependency "libffi"
  dependency "ncurses"
  dependency "zlib"
  dependency "openssl"
  dependency "bzip2"
  dependency "libsqlite3"
  dependency "liblzma"
  dependency "libyaml"

  source :url => "https://python.org/ftp/python/#{version}/Python-#{version}.tgz",
         :sha256 => "c7cfa39a43b994621b245e029769e9126caa2a93571cee2e743b213cceac35fb"

  relative_path "Python-#{version}"

  python_configure = ["./configure",
                      "--prefix=#{install_dir}/embedded"]

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

  if windows_arch_i386?
    dependency "vc_ucrt_redist"

    source :url => "http://s3.amazonaws.com/dd-agent-omnibus/python-windows-#{version}-x86.zip",
            :sha256 => "44f4a665392912fe172223c42c62dd193670392fee5010bc9f89c5cd2722964c"
  else

    # note that startring with 3.7.3 on Windows, the zip should be created without the built-in pip
    source :url => "https://s3.amazonaws.com/dd-agent-omnibus/python-windows-#{version}-amd64.zip",
         :sha256 => "58563ca60891025923572107e02b8f07439928eb5222dd10466cc92089072c2a"

  end
  build do
    command "XCOPY /YEHIR *.* \"#{windows_safe_path(python_3_embedded)}\""
  end
end
