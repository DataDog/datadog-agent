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
  mkdir "#{install_dir}/embedded/share/system-probe/java"

  copy 'pkg/network/java/agent-usm.jar', "#{install_dir}/embedded/share/system-probe/java/"

  if ENV.has_key?('SYSTEM_PROBE_BIN') and not ENV['SYSTEM_PROBE_BIN'].empty?
    copy "#{ENV['SYSTEM_PROBE_BIN']}/system-probe", "#{install_dir}/embedded/bin/system-probe"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/usm.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/usm-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/dns.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/dns-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/offset-guess.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/offset-guess-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/conntrack.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/conntrack-debug.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security-syscall-wrapper.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security-offset-guesser.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/oom-kill-co-re.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/oom-kill.o"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tcp-queue-length-co-re.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/tcp-queue-length.o"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/usm-co-re.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/usm.o"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/usm-debug-co-re.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/usm-debug.o"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/usm.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/conntrack.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/oom-kill.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tcp-queue-length.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/clang-bpf", "#{install_dir}/embedded/bin/clang-bpf"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/llc-bpf", "#{install_dir}/embedded/bin/llc-bpf"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/minimized-btfs.tar.xz", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/btf/minimized-btfs.tar.xz"
  end

  copy 'pkg/ebpf/c/COPYING', "#{install_dir}/embedded/share/system-probe/ebpf/"
end
