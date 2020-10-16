name "net-snmp"
default_version "5.9"

version "5.9" do
  source sha256: "2492dc334faf5ef26fe9783b9be6dfd489a256030883f05004963cc0dc903dfd"
end

source url: "https://ddintegrations.blob.core.windows.net/snmp/net-snmp-#{version}.tar.gz"

relative_path "net-snmp-#{version}"

build do
#   ship_license "./COPYING"
  env = with_standard_compiler_flags(with_embedded_path)

  configure_args = [
    "--prefix=#{install_dir}/embedded",
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

    # mibs and mibs-loading cannot be disable, needed for python3-netsnmp bindings
    # "--disable-mibs",
    # "--disable-mib-loading",
  ]

  if windows?
    configure_command = configure_args.unshift("sh configure").join(" ")
  else
    configure_command = configure_args.unshift("./configure").join(" ")
  end

  command configure_command, env: env
  make env: env
  make "install", env: env

  whitelist_file "#{install_dir}/embedded/lib/libnetsnmpmibs.so.40.0.0"
end
