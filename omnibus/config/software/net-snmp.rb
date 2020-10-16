name "net-snmp"
# default_version "5.7.3"
#
# version "5.7.3" do
#   source sha256: "12ef89613c7707dc96d13335f153c1921efc9d61d3708ef09f3fc4a7014fb4f0"
# end
#
# source url: "https://deac-ams.dl.sourceforge.net/project/net-snmp/net-snmp/#{version}/net-snmp-#{version}.tar.gz"
#
# relative_path "net-snmp-#{version}"

# default_version "5.9"
#
# version "5.9" do
#   source sha256: "2492dc334faf5ef26fe9783b9be6dfd489a256030883f05004963cc0dc903dfd"
# end
#
# source url: "https://ddintegrations.blob.core.windows.net/snmp/net-snmp-#{version}.tar.gz"
#
# relative_path "net-snmp-#{version}"

default_version "ed90aaaaea0d9cc6c5c5533f1863bae598d3b820"

version "ed90aaaaea0d9cc6c5c5533f1863bae598d3b820" do
  source sha256: "5cf1f605152c480abd549f543d05698fb32622a7a3f7dfcda7b649fbb804fd15"
end

source url: "https://github.com/net-snmp/net-snmp/archive/ed90aaaaea0d9cc6c5c5533f1863bae598d3b820.tar.gz"

relative_path "net-snmp-#{version}"

# default_version "5.9"
#
# source :url => "https://github.com/net-snmp/net-snmp/archive/ed90aaaaea0d9cc6c5c5533f1863bae598d3b820.zip",
#        :sha256 => "1d86261db919fca112fcc594ed881761c5b54ce372f97ceb3bc8a3a91ff68511",
#        :extract => :seven_zip

build do
#   ship_license "./COPYING"
  env = with_standard_compiler_flags(with_embedded_path)

  configure_args = [
    "--prefix=#{install_dir}/embedded",
    # "--enable-mini-agent",  # for a faster build
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
    # "--disable-mibs",
    # "--disable-mib-loading",
  ]

  configure_command = configure_args.unshift("./configure").join(" ")

  command configure_command, env: env
  make env: env
  make "install", env: env

  command ["ls", "-la", "#{install_dir}/embedded"]
#   command ["ls", "-la", "src/snmplib/.libs"]
#   copy "src/snmplib/.libs/libnetsnmp.so.35.0.0", "#{install_dir}/embedded/lib/libnetsnmp.so"
end
