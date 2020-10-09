name "net-snmp"
default_version "5.7.3"

version "5.7.3" do
  source sha256: "12ef89613c7707dc96d13335f153c1921efc9d61d3708ef09f3fc4a7014fb4f0"
end

source url: "https://deac-ams.dl.sourceforge.net/project/net-snmp/net-snmp/#{version}/net-snmp-#{version}.tar.gz"

relative_path "net-snmp-#{version}"

build do
  ship_license "./COPYING"
  env = with_standard_compiler_flags(with_embedded_path)

  configure_args = [
    "--prefix=#{install_dir}/embedded",
    "--enable-mini-agent",
  ]

  configure_command = configure_args.unshift("./configure").join(" ")

  command configure_command, env: env
  make env: env
  make "install", env: env

#   command ["ls", "-la", "src/snmplib/.libs"]
#   copy "src/snmplib/.libs/libnetsnmp.so.35.0.0", "#{install_dir}/embedded/lib/libnetsnmp.so"
end
