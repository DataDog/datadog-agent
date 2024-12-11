#include "kconfig.h"
#include "ktypes.h"
#include "bpf_helpers.h"
#include "bpf_builtins.h"
#include "bpf_metadata.h"

#include <uapi/linux/ptrace.h>

#include "defs.h"
#include "map-defs.h"
#include "protocols/events.h"

// --------------------------------------------------------
// this is a test program for pkg/networks/protocols/events
// --------------------------------------------------------

#define BATCH_SIZE 15

typedef struct {
     __u32 expected_pid;
     __u64 expected_fd;
     __u64 event_id;
} test_ctx_t;

BPF_LRU_MAP(test, __u32, test_ctx_t, 1);
USM_EVENTS_INIT(test, u64, BATCH_SIZE);

// source /sys/kernel/debug/tracing/events/syscalls/sys_enter_write/format
struct syscalls_enter_write_args {
    unsigned long long unused;
    long syscall_nr;
    unsigned long long fd;
    const char* buf;
    size_t count;
};

SEC("tracepoint/syscalls/sys_enter_write")
int tracepoint__syscalls__sys_enter_write(struct syscalls_enter_write_args *ctx) {
    __u32 zero = 0;
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    test_ctx_t *test_ctx = bpf_map_lookup_elem(&test, &zero);
    if (!test_ctx || test_ctx->expected_fd != ctx->fd || test_ctx->expected_pid != pid)
        return 0;

    // we're echoing to userspace whatever we read from the eBPF map
    __u64 event = test_ctx->event_id;

    // these functions are dynamically defined by `USM_EVENTS_INIT`
    test_batch_enqueue(&event);
    test_batch_flush((void*)ctx);
    return 0;
}

char _license[] SEC("license") = "GPL";
