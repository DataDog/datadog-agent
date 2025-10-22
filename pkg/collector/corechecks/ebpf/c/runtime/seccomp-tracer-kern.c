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

// source: include/uapi/linux/seccomp.h
#define SECCOMP_RET_KILL_PROCESS 0x80000000U /* kill the process */
#define SECCOMP_RET_KILL_THREAD 0x00000000U /* kill the thread */
#define SECCOMP_RET_KILL SECCOMP_RET_KILL_THREAD
#define SECCOMP_RET_TRAP 0x00030000U /* disallow and force a SIGSYS */
#define SECCOMP_RET_ERRNO 0x00050000U /* returns an errno */
#define SECCOMP_RET_USER_NOTIF 0x7fc00000U /* notifies userspace */
#define SECCOMP_RET_TRACE 0x7ff00000U /* pass to a tracer or disallow */
#define SECCOMP_RET_LOG 0x7ffc0000U /* allow after logging */
#define SECCOMP_RET_ALLOW 0x7fff0000U /* allow */

/* Masks for the return value sections. */
#define SECCOMP_RET_ACTION_FULL 0xffff0000U
#define SECCOMP_RET_ACTION 0x7fff0000U
#define SECCOMP_RET_DATA 0x0000ffffU

// CO-RE only: Read syscall number from task's pt_regs using bpf_task_pt_regs()
SEC("kretprobe/seccomp_run_filters")
int BPF_KRETPROBE(kretprobe__seccomp_run_filters, int ret) {
    u32 action = ret & SECCOMP_RET_ACTION_FULL;

    // Only track denials
    if (action == SECCOMP_RET_ALLOW || action == SECCOMP_RET_LOG || action == SECCOMP_RET_USER_NOTIF || action == SECCOMP_RET_TRACE) {
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
