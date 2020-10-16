name "net-snmp"
default_version "e87651cd12a37571f81772b345aefe2370ecc21f"

if windows?
    source url: "https://github.com/AlexandreYang/net-snmp/archive/#{version}.zip"
    source sha256: "fb2b77a57055ba2c0d7caebee91bb6c8d4d0fe7deb727f920325b8d35e7d13c4"
    source extract: :seven_zip
else
    source url: "https://github.com/AlexandreYang/net-snmp/archive/#{version}.tar.gz"
    source sha256: "d08fc4655b77361aad843731b6ec98480ec881dbbce037e753507d4007a4fc29"
end

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
