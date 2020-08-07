# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.

name 'system-probe'

if ENV['WITH_BCC'] == 'true'
  dependency 'libbcc'
end

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

build do
  if ENV.has_key?('SYSTEM_PROBE_BIN') and not ENV['SYSTEM_PROBE_BIN'].empty?
    copy ENV['SYSTEM_PROBE_BIN'], "#{install_dir}/embedded/bin/system-probe"
  end

  mkdir "#{install_dir}/embedded/share/system-probe/ebpf"

  copy 'pkg/ebpf/c/COPYING', "#{install_dir}/embedded/share/system-probe/ebpf/"
  copy 'pkg/ebpf/bytecode/tracer-ebpf.o', "#{install_dir}/embedded/share/system-probe/ebpf/"
  copy 'pkg/ebpf/bytecode/tracer-ebpf-debug.o', "#{install_dir}/embedded/share/system-probe/ebpf/"
  copy 'pkg/ebpf/c/oom-kill-kern.c', "#{install_dir}/embedded/share/system-probe/ebpf/"
  copy 'pkg/ebpf/c/tcp-queue-length-kern.c', "#{install_dir}/embedded/share/system-probe/ebpf/"

  copy 'pkg/security/ebpf/c/runtime-security.o', "#{install_dir}/embedded/share/system-probe/ebpf/"
  copy 'pkg/security/ebpf/c/runtime-security-syscall-wrapper.o', "#{install_dir}/embedded/share/system-probe/ebpf/"
end
