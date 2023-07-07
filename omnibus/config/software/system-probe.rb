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

  copy 'pkg/network/protocols/tls/java/agent-usm.jar', "#{install_dir}/embedded/share/system-probe/java/"

  if ENV.has_key?('SYSTEM_PROBE_BIN') and not ENV['SYSTEM_PROBE_BIN'].empty?
    copy "#{ENV['SYSTEM_PROBE_BIN']}/system-probe", "#{install_dir}/embedded/bin/system-probe"

    #
    # We would need to have the binary signature after strip pass, but didn't find a way to hook (or run later, but before the packager) the Stripper on omnibus
    #
    # WARNING
    # This is only works as expected as long as the omnibus Stripper class performs the same exact stripping operations as we would need the exact same binary.
    # https://github.com/DataDog/omnibus-ruby/blob/datadog-5.5.0/lib/omnibus/stripper.rb#L78-L94
    #
    #
    # The following part will be rewritten, as we would need agent, security-agent, process-agent and system-probe binary be available at the same place
    mkdir "#{install_dir}/.debug"
    mkdir "#{install_dir}/.debug/bin/agent"
    mkdir "#{install_dir}/.debug/embedded"
    mkdir "#{install_dir}/.debug/embedded/bin"

    # Update binary signature for unix socket connection when system_probe_config.sysprobe_auth_socket is true
    bak = "#{install_dir}/bin/agent/agent.bak"
    source = "#{install_dir}/bin/agent/agent"
    target = "#{install_dir}/.debug/bin/agent/agent.dbg"
    copy "#{source}", "#{bak}"
    command "objcopy --only-keep-debug #{source} #{target}"
    command "strip --strip-debug --strip-unneeded #{source}"
    command "objcopy --add-gnu-debuglink=#{target} #{source}"
    command "sha256sum -b #{source}"
    command "sed -i \"s|UDS_AGENT_SIG-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca4|$(sha256sum -b #{source} | sed 's| .*||g')|g\" #{install_dir}/embedded/bin/system-probe"
    delete "#{source}"
    move "#{bak}", "#{source}"

    bak = "#{install_dir}/embedded/bin/process-agent.bak"
    source = "#{install_dir}/embedded/bin/process-agent"
    target = "#{install_dir}/.debug/embedded/bin/process-agent.dbg"
    copy "#{source}", "#{bak}"
    command "objcopy --only-keep-debug #{source} #{target}"
    command "strip --strip-debug --strip-unneeded #{source}"
    command "objcopy --add-gnu-debuglink=#{target} #{source}"
    command "sha256sum -b #{source}"
    command "sed -i \"s|UDS_PROCESS_AGENT_SIG-6df08279acf372b0fe1c624369059fe2d6ade65d05|$(sha256sum -b #{source} | sed 's| .*||g')|g\" #{install_dir}/embedded/bin/system-probe"
    delete "#{source}"
    move "#{bak}", "#{source}"

    # Update binary signature for unix socket connection when runtime_security_config.auth_socket is true
    bak = "#{install_dir}/embedded/bin/security-agent.bak"
    source = "#{install_dir}/embedded/bin/security-agent"
    target = "#{install_dir}/.debug/embedded/bin/security-agent.dbg"
    copy "#{source}", "#{bak}"
    command "objcopy --only-keep-debug #{source} #{target}"
    command "strip --strip-debug --strip-unneeded #{source}"
    command "objcopy --add-gnu-debuglink=#{target} #{source}"
    command "sha256sum -b #{source}"
    command "sed -i \"s|UDS_SECURITY_AGENT_SIG-4ce7aa6ef3c376b3d80ac1ec5f2b50fcd5d65e896|$(sha256sum -b #{source} | sed 's| .*||g')|g\" #{install_dir}/embedded/bin/system-probe"
    delete "#{source}"
    move "#{bak}", "#{source}"

    command "sha256sum -b #{install_dir}/embedded/bin/*"
    delete "#{install_dir}/.debug"

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
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer-co-re.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/tracer.o"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer-debug-co-re.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/tracer-debug.o"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer-fentry-co-re.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/tracer-fentry.o"
    copy "#{ENV['SYSTEM_PROBE_BIN']}/tracer-fentry-debug-co-re.o", "#{install_dir}/embedded/share/system-probe/ebpf/co-re/tracer-fentry-debug.o"
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
