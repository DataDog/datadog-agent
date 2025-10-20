#include "ktypes.h"
#include "bpf_metadata.h"


#include "seccomp-tracer-kern-user.h"
#include "cgroup.h"

#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "bpf_telemetry.h"

/*
 * Ring buffer for sending seccomp denial events to userspace
 * Size is configured from userspace via ResizeRingBuffer
 */
BPF_RINGBUF_MAP(seccomp_events, seccomp_event_t)

// CO-RE only: Read syscall number from task's pt_regs using bpf_task_pt_regs()
SEC("kretprobe/__seccomp_filter")
int BPF_KRETPROBE(kretprobe____seccomp_filter, int ret)
{
    // The return value contains the seccomp action
    // SECCOMP_RET_KILL_PROCESS = 0x80000000
    // SECCOMP_RET_KILL_THREAD  = 0x00000000
    // SECCOMP_RET_TRAP         = 0x00030000
    // SECCOMP_RET_ERRNO        = 0x00050000
    // SECCOMP_RET_ALLOW        = 0x7fff0000

    u32 action = ret & 0xffff0000;

    // Only track denials (not ALLOW)
    if (action == 0x7fff0000) { // SECCOMP_RET_ALLOW
        return 0;
    }

    // Read syscall number from the task's pt_regs
    // This is the syscall number that the task was trying to execute
    struct pt_regs *regs = (struct pt_regs *)bpf_task_pt_regs(bpf_get_current_task_btf());
    if (!regs) {
        return 0;
    }

    int syscall_nr;
#if defined(__TARGET_ARCH_arm64)
    syscall_nr = BPF_CORE_READ(regs, syscallno);
#elif defined(__TARGET_ARCH_x86)
    syscall_nr = BPF_CORE_READ(regs, orig_ax);
#else
    // Fallback: this might not work on all architectures
    syscall_nr = -1;
#endif

    if (syscall_nr < 0) {
        return 0;
    }

    // Build and send event to ring buffer
    seccomp_event_t event = { 0 };

    if (!get_cgroup_name(event.cgroup, sizeof(event.cgroup))) {
        return 0;
    }

    event.syscall_nr = (u32)syscall_nr;
    event.action = action;

    bpf_ringbuf_output_with_telemetry(&seccomp_events, &event, sizeof(event), 0);

    return 0;
}

char _license[] SEC("license") = "GPL";
