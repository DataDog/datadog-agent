# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'libelf'
default_version '0.178'

version '0.178' do
  source url: 'https://sourceware.org/elfutils/ftp/0.178/elfutils-0.178.tar.bz2',
         sha512: '356656ad0db8f6877b461de1a11280de16a9cc5d8dde4381a938a212e828e32755135e5e3171d311c4c9297b728fbd98123048e2e8fbf7fe7de68976a2daabe5'
end

dependency 'zlib'

relative_path "elfutils-#{version}"

build do
  command %q(patch -p 1 <<"EOF"
--- elfutils-0.178/src/elfclassify.c	2019-11-26 22:48:42.000000000 +0000
+++ elfutils-0.178.patched/src/elfclassify.c	2020-01-28 09:22:28.066520000 +0000
@@ -827,7 +827,10 @@
       break;
     case do_print0:
       if (checks_passed == flag_print_matching)
+#pragma GCC diagnostic push
+#pragma GCC diagnostic ignored "-Wunused-result"
         fwrite (current_path, strlen (current_path) + 1, 1, stdout);
+#pragma GCC diagnostic pop
       break;
     case no_print:
       if (!checks_passed)
EOF
)
  env = {
    "CFLAGS" => "-I#{install_dir}/embedded/include -O2 -pipe",
    "LDFLAGS" => "-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib",
  }
  command "./configure" \
          " --prefix=#{install_dir}/embedded" \
          " --disable-static",
          " --disable-debuginfod" \
          " --disable-dependency-tracking", :env => env
  make "-j #{workers}", :env => env
  make 'install', :env => env
  delete "#{install_dir}/embedded/bin/eu-*"
end
