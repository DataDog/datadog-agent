name "net-snmp"
default_version "5.8"

version "5.8" do
  source sha256: "b5d4938d3a86eebb858de4e367fead2e7eedda33468994f5e38db3a9e8339f74"
end

source url: "https://deac-ams.dl.sourceforge.net/project/net-snmp/net-snmp/#{version}/net-snmp-#{version}.tar.gz"

relative_path "net-snmp-#{version}"

build do
  ship_license "./COPYING"
  env = with_standard_compiler_flags(with_embedded_path)

  configure_args = [
    "--disable-readline",
  ]

  configure_command = configure_args.unshift("./configure").join(" ")

  command configure_command, env: env, in_msys_bash: true
  make env: env

  command ["ls", "-la", "src/snmplib/.libs"]
  copy "src/snmplib/.libs/libnetsnmp.so.35.0.0", "#{install_dir}/embedded/lib/libnetsnmp.so"
end
