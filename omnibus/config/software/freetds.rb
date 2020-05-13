name "freetds"
default_version "1.1.36"

version "1.1.36" do
  source sha256: "1e7b6cc03d0a0184141cd90766dcd93cf4fa6a5b8d2142d37ada02addaf946b7"
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
end
