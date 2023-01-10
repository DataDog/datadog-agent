name "freetds"
default_version "1.1.36"

version "1.1.36" do
  source sha256: "1c306e658e10a325eefddfd662cec3a6d9065fe61c515f26d4f1fb6c4c62405d"
end

source url: "https://www.freetds.org/files/stable/freetds-#{version}.tar.gz"

relative_path "freetds-#{version}"

build do
  license "LGPL-2.1"
  license_file "./COPYING"
  license_file "./COPYING.lib"

  env = with_standard_compiler_flags(with_embedded_path)

  configure_args = [
    "--disable-readline",
    "--disable-static",
  ]

  configure_command = configure_args.unshift("./configure").join(" ")

  command configure_command, env: env, in_msys_bash: true
  command "make -j #{workers}", env: env

  # Only `libtdsodbc.so/libtdsodbc.so.0.0.0` are needed for SQLServer integration.
  # Hence we only need to copy those.
  copy "src/odbc/.libs/libtdsodbc.so", "#{install_dir}/embedded/lib/libtdsodbc.so"
  copy "src/odbc/.libs/libtdsodbc.so.0.0.0", "#{install_dir}/embedded/lib/libtdsodbc.so.0.0.0"

end
