# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'system-probe'

source path: '..'
relative_path 'src/github.com/DataDog/datadog-agent'

always_build true

build do
  license :project_license

  mkdir "#{install_dir}/embedded/share/system-probe/ebpf"
  mkdir "#{install_dir}/embedded/share/system-probe/ebpf/runtime"
  mkdir "#{install_dir}/embedded/share/system-probe/ebpf/co-re"
  # ensure previous agent version extracted BTFs are removed
  delete "#{install_dir}/embedded/share/system-probe/ebpf/co-re/btf"
  mkdir "#{install_dir}/embedded/share/system-probe/ebpf/co-re/btf"
  mkdir "#{install_dir}/embedded/share/system-probe/java"

  if ENV.has_key?('SYSTEM_PROBE_BIN') and not ENV['SYSTEM_PROBE_BIN'].empty?
    copy "pkg/ebpf/bytecode/build/*.o", "#{install_dir}/embedded/share/system-probe/ebpf/"
    delete "#{install_dir}/embedded/share/system-probe/ebpf/usm_events_test*.o"
    copy "pkg/ebpf/bytecode/build/co-re/*.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/"
    copy "pkg/ebpf/bytecode/build/runtime/*.c", "#{install_dir}/embedded/share/system-probe/ebpf/runtime/"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/clang-bpf", "#{install_dir}/embedded/bin/clang-bpf"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/llc-bpf", "#{install_dir}/embedded/bin/llc-bpf"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/minimized-btfs.tar.xz", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/btf/minimized-btfs.tar.xz"
  end

  copy 'pkg/ebpf/c/COPYING', "#{install_dir}/embedded/share/system-probe/ebpf/"
end
