name "cyrus-sasl"
default_version "2.1.28"

# test if lmdb can replace gdbm
# dependency "gdbm"
dependency "lmdb"

if redhat?
    #dependency "libcom_err"
    dependency "e2fsprogs"
end

source :url => "https://github.com/cyrusimap/cyrus-sasl/releases/download/cyrus-sasl-#{version}/cyrus-sasl-#{version}.tar.gz",
       :sha256 => "7ccfc6abd01ed67c1a0924b353e526f1b766b21f42d4562ee635a8ebfc5bb38c",
       :extract => :seven_zip

relative_path "cyrus-sasl-#{version}"

build do

  env = {
    "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
    "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
    "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
  }

  configure_command = ["./configure",
                        "--prefix=#{install_dir}/embedded",
                        "--with-dblib=lmdb"]

  if osx?
    # https://github.com/Homebrew/homebrew-core/blob/e2071268473bcddaf72f8e3f7aa4153a18d1ccfa/Formula/cyrus-sasl.rb
    configure_command = configure_command.append("--disable-macos-framework")
  end

  command configure_command.join(" "), env: env
  command "make", :env => env
  command "make install", :env => env

end
