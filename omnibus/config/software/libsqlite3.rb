name "libsqlite3"
default_version "3.43.1"

dependency "config_guess"
dependency "zlib"

source url: "https://www.sqlite.org/2023/sqlite-autoconf-3430101.tar.gz",
       sha256: "098984eb36a684c90bc01c0eb7bda3273c327cbc3673d7d0bc195028c19fb7b0"

relative_path "sqlite-autoconf-3430100"

build do
  license "Public-Domain"

  update_config_guess
  env = with_standard_compiler_flags(with_embedded_path)
  configure_options = [
    "--disable-nls",
    "--disable-static",
    "--enable-shared",
    "--enable-pic",
    "--disable-editline",
    "--disable-readline",
  ]
  configure(*configure_options, env: env)
  make "-j #{workers}", env: env
  make "install"
  delete "#{install_dir}/embedded/bin/sqlite3"
end
