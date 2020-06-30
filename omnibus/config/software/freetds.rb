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
    "--prefix=#{install_dir}/embedded",
  ]

  configure_command = configure_args.unshift("./configure").join(" ")

  command configure_command, env: env, in_msys_bash: true
  make env: env
  make "install", env: env

  # removing unused files
  delete "#{install_dir}/embedded/bin/tsql"
  delete "#{install_dir}/embedded/bin/fisql"
end
