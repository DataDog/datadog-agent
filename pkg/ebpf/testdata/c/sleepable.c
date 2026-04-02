#include "bpf_metadata.h"
#include "bpf_tracing.h"

SEC("fexit/__x64_sys_open")
int BPF_PROG(fexit__x64_sys_open, const struct pt_regs* regs, long ret) {
    const char* pathname;
    char buf[16];

    bpf_probe_read_kernel(&pathname, sizeof(pathname), &PT_REGS_PARM1(regs));
    bpf_copy_from_user(&buf, sizeof(buf), pathname);

    return 0;
}

char _license[] SEC("license") = "GPL";
