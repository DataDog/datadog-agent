# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'system-probe'

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  license :project_license

  mkdir "#{install_dir}/embedded/share/system-probe/ebpf"
  mkdir "#{install_dir}/embedded/share/system-probe/ebpf/runtime"
  mkdir "#{install_dir}/embedded/share/system-probe/ebpf/co-re"
  mkdir "#{install_dir}/embedded/share/system-probe/ebpf/co-re/btf"
  mkdir "#{install_dir}/embedded/nikos/embedded/bin"
  mkdir "#{install_dir}/embedded/nikos/embedded/lib"

  if ENV.has_key?('SYSTEM_PROBE_BIN') and not ENV['SYSTEM_PROBE_BIN'].empty?
    copy "#{ENV['SYSTEM_PROBE_BIN']}/system-probe", "#{install_dir}/embedded/bin/system-probe"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/http.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/http-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/dns.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/dns-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/offset-guess.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/offset-guess-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security-syscall-wrapper.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security-offset-guesser.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/http.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/conntrack.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/oom-kill.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tcp-queue-length.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/clang-bpf", "#{install_dir}/embedded/bin/clang-bpf"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/llc-bpf", "#{install_dir}/embedded/bin/llc-bpf"
    command "tar -xf #{ENV['SYSTEM_PROBE_BIN']}/minimized-btfs.tar.gz -C #{install_dir}/embedded/share/system-probe/ebpf/co-re/btf/"
  end

  copy 'pkg/ebpf/c/COPYING', "#{install_dir}/embedded/share/system-probe/ebpf/"

  if ENV.has_key?('NIKOS_PATH') and not ENV['NIKOS_PATH'].empty?
    copy "#{ENV['NIKOS_PATH']}/bin/gpg", "#{install_dir}/embedded/nikos/embedded/bin/"
    copy "#{ENV['NIKOS_PATH']}/lib/rpm", "#{install_dir}/embedded/nikos/embedded/lib/"
    delete "#{install_dir}/embedded/nikos/embedded/lib/rpm/debugedit"
    delete "#{install_dir}/embedded/nikos/embedded/lib/rpm/elfdeps"
    delete "#{install_dir}/embedded/nikos/embedded/lib/rpm/rpmdeps"
    delete "#{install_dir}/embedded/nikos/embedded/lib/rpm/sepdebugcrcfix"
    copy "#{ENV['NIKOS_PATH']}/lib/libreadline.so", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libreadline.so.8", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libreadline.so.8.0", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libncursesw.so", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libncursesw.so.5", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libncursesw.so.5.9", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libtinfow.so", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libtinfow.so.5", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libtinfow.so.5.9", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libtinfo.so", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libtinfo.so.5", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/lib/libtinfo.so.5.9", "#{install_dir}/embedded/nikos/embedded/lib/"
    copy "#{ENV['NIKOS_PATH']}/ssl", "#{install_dir}/embedded/nikos/embedded/"
  end
end
