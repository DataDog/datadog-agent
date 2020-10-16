name "net-snmp"
default_version "ed90aaaaea0d9cc6c5c5533f1863bae598d3b820"

version "ed90aaaaea0d9cc6c5c5533f1863bae598d3b820" do
  source sha256: "5cf1f605152c480abd549f543d05698fb32622a7a3f7dfcda7b649fbb804fd15"
end

source url: "https://github.com/net-snmp/net-snmp/archive/ed90aaaaea0d9cc6c5c5533f1863bae598d3b820.tar.gz"

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
  whitelist_file "#{install_dir}/embedded/lib/libnetsnmpagent.so.40.0.0"
end
