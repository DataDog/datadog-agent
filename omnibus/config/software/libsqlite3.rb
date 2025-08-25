name "libsqlite3"
default_version "3.50.4"

dependency "config_guess"
dependency "zlib"

source url: "https://sqlite.org/2025/sqlite-autoconf-3500400.tar.gz",
       sha256: "a3db587a1b92ee5ddac2f66b3edb41b26f9c867275782d46c3a088977d6a5b18"

relative_path "sqlite-autoconf-3500400"

build do
  license "Public-Domain"

  update_config_guess
  env = with_standard_compiler_flags(with_embedded_path)
  configure_options = [
    "--disable-static",
    "--enable-shared",
    "--disable-editline",
    "--disable-readline",
  ]
  configure(*configure_options, env: env)
  make "-j #{workers}", env: env
  make "install"
  delete "#{install_dir}/embedded/bin/sqlite3"
end
