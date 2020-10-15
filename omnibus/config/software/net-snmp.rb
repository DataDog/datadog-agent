name "net-snmp"
# default_version "5.9"
#
# version "5.9" do
#   source sha256: "04303a66f85d6d8b16d3cc53bde50428877c82ab524e17591dfceaeb94df6071"
# end
#
# source url: "https://deac-ams.dl.sourceforge.net/project/net-snmp/net-snmp/#{version}/net-snmp-#{version}.tar.gz"

default_version "5.9"

source :url => "https://github.com/net-snmp/net-snmp/archive/ed90aaaaea0d9cc6c5c5533f1863bae598d3b820.zip"
       :sha256 => "1d86261db919fca112fcc594ed881761c5b54ce372f97ceb3bc8a3a91ff68511",
       :extract => :seven_zip

relative_path "net-snmp-#{version}"

build do
  ship_license "./COPYING"
  env = with_standard_compiler_flags(with_embedded_path)

  configure_args = [
    "--prefix=#{install_dir}/embedded",
    "--enable-mini-agent",  # for a faster build
    "--with-default-snmp-version=2",
    "--with-sys-contact=contact",
    "--with-sys-location=Unknown",
    "--with-logfile=/var/log/snmpd.log",
    "--with-persistent-directory=/var/net-snmp",
    "--without-perl-modules",
    "--disable-embedded-perl",
    "--disable-agent",
    "--disable-applications",
    "--disable-manuals",
    "--disable-scripts",
    "--disable-mibs",
    "--disable-mib-loading",
  ]

  configure_command = configure_args.unshift("./configure").join(" ")

  command configure_command, env: env
  make env: env
  make "install", env: env

  command ["ls", "-la", "#{install_dir}/embedded"]
#   command ["ls", "-la", "src/snmplib/.libs"]
#   copy "src/snmplib/.libs/libnetsnmp.so.35.0.0", "#{install_dir}/embedded/lib/libnetsnmp.so"
end
