# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "libpcap"
default_version "1.10.5"
# system-probe doesn't depend on any particular version of libpcap so use the latest one (as of 2024-10-28)
# this version should be kept in sync with the one in tasks/system_probe.py
version "1.10.5" do
  source sha256: "84fa89ac6d303028c1c5b754abff77224f45eca0a94eb1a34ff0aa9ceece3925"
end

license "BSD-3-Clause"
license_file "LICENSE"

relative_path "libpcap-#{version}"

source url: "https://www.tcpdump.org/release/libpcap-#{version}.tar.xz"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  configure_options = [
    "--disable-largefile",
    "--disable-instrument-functions",
    "--disable-remote",
    "--disable-usb",
    "--disable-netmap",
    "--disable-bluetooth",
    "--disable-dbus",
    "--disable-rdma",
  ]
  configure(*configure_options, env: env)

  make "-j #{workers}", env: env
  make "install", env: env

  delete "#{install_dir}/embedded/bin/pcap-config"
  delete "#{install_dir}/embedded/lib/libpcap.a"

end
