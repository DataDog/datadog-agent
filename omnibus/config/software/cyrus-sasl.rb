name "cyrus-sasl"
default_version "2.1.28"

dependency "lmdb"

if redhat?
    dependency "libcom_err"
end

source :url => "https://github.com/cyrusimap/cyrus-sasl/releases/download/cyrus-sasl-#{version}/cyrus-sasl-#{version}.tar.gz",
       :sha256 => "7ccfc6abd01ed67c1a0924b353e526f1b766b21f42d4562ee635a8ebfc5bb38c",
       :extract => :seven_zip

relative_path "cyrus-sasl-#{version}"

build do
  license "Carnegie Mellon University license"
  license_file "https://raw.githubusercontent.com/cyrusimap/cyrus-sasl/master/COPYING"

  env = with_standard_compiler_flags(with_embedded_path)

  configure_opts = ["--with-dblib=lmdb"]

  if osx_target?
    # https://github.com/Homebrew/homebrew-core/blob/e2071268473bcddaf72f8e3f7aa4153a18d1ccfa/Formula/cyrus-sasl.rb
    configure_opts = configure_opts.append("--disable-macos-framework")
  end

  configure(*configure_opts, env: env)
  command "make", :env => env
  command "make install", :env => env

end
