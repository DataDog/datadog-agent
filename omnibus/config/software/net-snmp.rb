name "net-snmp"
default_version "239317b8040850ac530905c652088ffe37c1a991"

if windows?
    source url: "https://github.com/AlexandreYang/net-snmp/archive/#{version}.zip"
    source sha256: "91222a869eb61aa7ca9732deda5a576d27ffdee5b6582ed3079b065670097ec1"
    source extract: :seven_zip
else
    source url: "https://github.com/AlexandreYang/net-snmp/archive/#{version}.tar.gz"
    source sha256: "09377d8fe5bdfa8f563c5a355f946f3916c79910308ebcd2824834a956519a67"
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

  if windows?
    # TODO: Use build env vars like this instead:
    #       https://github.com/DataDog/datadog-agent/blob/892bd315ec45bf9ecfa40a3ba602e15e39fb75f6/omnibus/config/software/datadog-agent-integrations-py3.rb#L140-L146
    copy "include/net-snmp", "#{install_dir}/embedded3/include/net-snmp"
    copy "win32/net-snmp/net-snmp-config.h", "#{install_dir}/embedded3/include/net-snmp/net-snmp-config.h"
  end

  whitelist_file "#{install_dir}/embedded/lib/libnetsnmpmibs.so.40.0.0"
  whitelist_file "#{install_dir}/embedded/lib/libnetsnmpagent.so.40.0.0"
end
