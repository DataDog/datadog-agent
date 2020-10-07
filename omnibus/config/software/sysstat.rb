# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

name "sysstat"
default_version "12.4.0"
skip_transitive_dependency_licensing true

ship_source true

source :url => "https://github.com/sysstat/sysstat/archive/v#{version}.tar.gz",
       :sha256 => "1962ed04832d798f5f7e787b9b4caa8b48fe935e854eef5528586b65f3cdd993"

relative_path "sysstat-#{version}"

env = {
  "LDFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
  "CFLAGS" => "-L#{install_dir}/embedded/lib -I#{install_dir}/embedded/include",
  "LD_RUN_PATH" => "#{install_dir}/embedded/lib",
  # tell the Makefile which is the directory containing config files by setting
  # `conf_dir`, otherwise `make install` will write to `/etc/`
  "conf_dir" =>  "#{install_dir}/embedded/etc"
}

build do
  ship_license "https://raw.githubusercontent.com/sysstat/sysstat/master/COPYING"
  command(["./configure",
       "--prefix=#{install_dir}/embedded",
       "--disable-nls",
       "--disable-sensors",
       "--disable-documentation"
    ].join(" "), :env => env)
  command "make -j #{workers}", :env => { "LD_RUN_PATH" => "#{install_dir}/embedded/lib" }
  command "make install"
end
