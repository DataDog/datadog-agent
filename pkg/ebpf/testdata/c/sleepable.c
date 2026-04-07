#include "bpf_metadata.h"
#include "bpf_tracing.h"
#include "bpf_telemetry.h"

#include "ktypes.h"

#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#include <linux/ptrace.h>
#endif

SEC("fexit/__x64_sys_open")
int BPF_PROG(test_modifier_x64, const struct pt_regs* regs, long ret) {
    const char* pathname;
    char buf[16];

    bpf_probe_read_kernel(&pathname, sizeof(pathname), &PT_REGS_PARM1(regs));
    bpf_copy_from_user(&buf, sizeof(buf), pathname);

    return 0;
}

SEC("fexit/__arm64_sys_open")
int BPF_PROG(test_modifier_arm64, struct pt_regs* regs, long ret) {
    const char* pathname;
    char buf[16];

    bpf_probe_read_kernel(&pathname, sizeof(pathname), &PT_REGS_PARM1(regs));
    bpf_copy_from_user(&buf, sizeof(buf), pathname);

    return 0;
}

SEC("fexit/__x64_sys_open")
int BPF_PROG(test_replaced_x64, const struct pt_regs* regs, long ret) {
    const char* pathname;
    char buf[16];

    bpf_probe_read_kernel(&pathname, sizeof(pathname), &PT_REGS_PARM1(regs));
    bpf_probe_read_user(&buf, sizeof(buf), pathname);

    return 0;
}

SEC("fexit/__arm64_sys_open")
int BPF_PROG(test_replaced_arm64, const struct pt_regs* regs, long ret) {
    const char* pathname;
    char buf[16];

    bpf_probe_read_kernel(&pathname, sizeof(pathname), &PT_REGS_PARM1(regs));
    bpf_probe_read_user(&buf, sizeof(buf), pathname);

    return 0;
}

SEC("fexit/__x64_sys_open")
int BPF_PROG(test_womodifier_x64, const struct pt_regs* regs, long ret) {
    const char* pathname;
    char buf[16];

    bpf_probe_read_kernel(&pathname, sizeof(pathname), &PT_REGS_PARM1(regs));
    bpf_copy_from_user(&buf, sizeof(buf), pathname);

    return 0;
}

SEC("fexit/__arm64_sys_open")
int BPF_PROG(test_womodifier_arm64, struct pt_regs* regs, long ret) {
    const char* pathname;
    char buf[16];

    bpf_probe_read_kernel(&pathname, sizeof(pathname), &PT_REGS_PARM1(regs));
    bpf_copy_from_user(&buf, sizeof(buf), pathname);

    return 0;
}

SEC("fexit/__x64_sys_openat")
int BPF_PROG(test_telemetry_x64, const struct pt_regs* regs, long ret) {
    char buf[16];
    bpf_probe_read_user_with_telemetry(&buf, sizeof(buf), (void *)0xdeadbeef);
    return 0;
}


SEC("fexit/__arm64_sys_openat")
int BPF_PROG(test_telemetry_arm64, struct pt_regs* regs, long ret) {
    char buf[16];
    bpf_probe_read_user_with_telemetry(&buf, sizeof(buf), (void *)0xdeadbeef);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_open")
int test_tracepoint(void *ctx) {
    return 0;
}

char _license[] SEC("license") = "GPL";
