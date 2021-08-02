# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'system-probe'

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  mkdir "#{install_dir}/embedded/share/system-probe/ebpf"
  mkdir "#{install_dir}/embedded/share/system-probe/ebpf/runtime"

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
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/runtime-security.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/conntrack.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
  end

  copy 'pkg/ebpf/c/COPYING', "#{install_dir}/embedded/share/system-probe/ebpf/"
  copy 'pkg/collector/corechecks/ebpf/c/runtime/bpf-common.h', "#{install_dir}/embedded/share/system-probe/ebpf/"

  copy 'pkg/collector/corechecks/ebpf/c/runtime/oom-kill-kern.c', "#{install_dir}/embedded/share/system-probe/ebpf/"
  copy 'pkg/collector/corechecks/ebpf/c/runtime/oom-kill-kern-user.h', "#{install_dir}/embedded/share/system-probe/ebpf/"

  copy 'pkg/collector/corechecks/ebpf/c/runtime/tcp-queue-length-kern.c', "#{install_dir}/embedded/share/system-probe/ebpf/"
  copy 'pkg/collector/corechecks/ebpf/c/runtime/tcp-queue-length-kern-user.h', "#{install_dir}/embedded/share/system-probe/ebpf/"
end
