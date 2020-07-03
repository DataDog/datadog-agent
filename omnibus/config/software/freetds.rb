name "freetds"
default_version "1.1.36"

version "1.1.36" do
  source sha256: "1c306e658e10a325eefddfd662cec3a6d9065fe61c515f26d4f1fb6c4c62405d"
end

source url: "ftp://ftp.freetds.org/pub/freetds/stable/freetds-#{version}.tar.gz"

relative_path "freetds-#{version}"

build do
  ship_license "./COPYING"
  env = with_standard_compiler_flags(with_embedded_path)

  configure_args = [
    "--disable-readline",
  ]

  configure_command = configure_args.unshift("./configure").join(" ")

  command configure_command, env: env, in_msys_bash: true
  make env: env

  # Only `libtdsodbc.so.0.0.0` is needed for SQLServer integration.
  # `libtdsodbc.so` and `libtdsodbc.so.0` symlinks are automatically created
  # Hence we only need to copy this file.
  copy "src/odbc/.libs/libtdsodbc.so.0.0.0", "#{install_dir}/embedded/lib/libtdsodbc.so.0.0.0"

end
