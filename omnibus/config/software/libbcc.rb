# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

name 'bcc'
default_version 'v0.12.0'

dependency 'libelf'
dependency 'libc++'

source git: 'https://github.com/iovisor/bcc.git'

relative_path 'bcc'

build do
  command %q(patch -p 1 <<<$'
diff --git a/introspection/bps.c b/introspection/bps.c
index 5916093..2689010 100644
--- a/introspection/bps.c
+++ b/introspection/bps.c
@@ -114,6 +114,8 @@ static void print_prog_hdr(void)

 static void print_prog_info(const struct bpf_prog_info *prog_info)
 {
+  printf("Disabled because `clock_gettime` causes link issue on old systems");
+#if 0
   struct timespec real_time_ts, boot_time_ts;
   time_t wallclock_load_time = 0;
   char unknown_prog_type[16];
@@ -152,6 +154,7 @@ static void print_prog_info(const struct bpf_prog_info *prog_info)
     printf("%8u- %-15s %8u %6u %-12s %-15s\\\\n",
            prog_info->id, prog_type, prog_info->created_by_uid,
            prog_info->nr_map_ids, load_time, prog_info->name);
+#endif
 }

 static void print_map_hdr(void)')
  command "cmake . -DCMAKE_INSTALL_PREFIX=#{install_dir}/embedded -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ -DCMAKE_CXX_FLAGS=-stdlib=libc++ -DCMAKE_CXX_FLAGS=-I#{install_dir}/embedded/include/c++/v1 -DCMAKE_EXE_LINKER_FLAGS='-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib -ltinfo -stdlib=libc++' -DCMAKE_SHARED_LINKER_FLAGS='-Wl,-rpath,#{install_dir}/embedded/lib -L#{install_dir}/embedded/lib -ltinfo -stdlib=libc++'"
  make "-j #{workers}"
  make 'install'
end
