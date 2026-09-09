#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "preempt.h"

/*
 * Single entry array holding the value last returned by get_nesting_depth().
 * One of TASK_DEPTH/SOFTIRQ_DEPTH/HARDIRQ_DEPTH/NMI_DEPTH, or negative if the
 * preempt count could not be read on this arch/kernel.
 */
BPF_ARRAY_MAP(nesting_depth, s32, 1)

static __always_inline int record_nesting_depth(void) {
    u32 key = 0;
    s32 *val = bpf_map_lookup_elem(&nesting_depth, &key);
    if (!val)
        return 0;

    *val = get_nesting_depth();

    return 0;
}

SEC("raw_tracepoint/sys_enter")
int raw_tracepoint__sys_enter(void *ctx) {
    return record_nesting_depth();
}

SEC("raw_tracepoint/softirq_entry")
int raw_tracepoint__softirq_entry(void *ctx) {
    return record_nesting_depth();
}

SEC("raw_tracepoint/local_timer_entry")
int raw_tracepoint__local_timer_entry(void *ctx) {
    return record_nesting_depth();
}

SEC("raw_tracepoint/irq_handler_entry")
int raw_tracepoint__irq_handler_entry(void *ctx) {
    return record_nesting_depth();
}

SEC("raw_tracepoint/nmi_handler")
int raw_tracepoint__nmi_handler(void *ctx) {
    return record_nesting_depth();
}

char __license[] SEC("license") = "GPL";
