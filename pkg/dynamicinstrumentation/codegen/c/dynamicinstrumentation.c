#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "kconfig.h"
#include <asm/ptrace.h>
#include "base_event.h"
#include "macros.h"
#include "event.h"
#include "maps.h"
#include "expressions.h"

SEC("uprobe/{{.GetBPFFuncName}}")
int {{.GetBPFFuncName}}(struct pt_regs *ctx)
{
    bpf_printk("{{.GetBPFFuncName}} probe in {{.ServiceName}} has triggered");

    __u16 collectionLimit = 0;

    // reserve space on ringbuffer
    struct event *event;
    event = bpf_ringbuf_reserve(&events, sizeof(struct event), 0);
    if (!event) {
        bpf_printk("No space available on ringbuffer, dropping event");
        return 0;
    }

    char* zero_string;
    __u32 key = 0;
    zero_string = bpf_map_lookup_elem(&zeroval, &key);
    if (!zero_string) {
        bpf_printk("couldn't lookup zero value in zeroval array map, dropping event for {{.GetBPFFuncName}}");
        bpf_ringbuf_discard(event, 0);
        return 0;
    }

    bpf_probe_read(&event->base.probe_id, sizeof(event->base.probe_id), zero_string);
    bpf_probe_read(&event->base.program_counters, sizeof(event->base.program_counters), zero_string);
    bpf_probe_read(&event->output, sizeof(event->output), zero_string);
    bpf_probe_read(&event->base.probe_id, {{ .ID | len }}, "{{.ID}}");

    // Get tid and tgid
    u64 pidtgid = bpf_get_current_pid_tgid();
    u32 tgid = pidtgid >> 32;
    event->base.pid = tgid;

    u64 uidgid = bpf_get_current_uid_gid();
    u32 uid = uidgid >> 32;
    event->base.uid = uid;

    // Collect stack trace
    __u64 currentPC = PT_REGS_IP(ctx);
    bpf_probe_read(&event->base.program_counters[0], sizeof(__u64), &currentPC);

    __u64 bp = PT_REGS_FP(ctx);
    bpf_probe_read(&bp, sizeof(__u64), (void*)bp); // dereference bp to get current stack frame
    __u64 ret_addr = PT_REGS_RET(ctx); // when bpf prog enters, the return address hasn't yet been written to the stack

    int i;
    int j;
    __u16 n;
    for (i = 1; i < STACK_DEPTH_LIMIT; i++)
    {
        if (bp == 0) {
            break;
        }
        bpf_probe_read(&event->base.program_counters[i], sizeof(__u64), &ret_addr);
        bpf_probe_read(&ret_addr, sizeof(__u64), (void*)(bp-8));
        bpf_probe_read(&bp, sizeof(__u64), (void*)bp);
    }

    // Collect parameters
    __u8 param_type;
    __u16 param_size;
    __u16 slice_length;

    int chunk_size = 0;
    int outputOffset = 0;

    __u64 *temp_storage = bpf_map_lookup_elem(&temp_storage_array, &key) ;
    if (!temp_storage) {
        bpf_ringbuf_discard(event, 0);
        return 0;
    }

    struct expression_context context = {
        .ctx = ctx,
        .output_offset = &outputOffset,
        .event = event,
        .limit = &collectionLimit,
        .temp_storage = temp_storage,
        .zero_string = zero_string
    };

    {{ .InstrumentationInfo.BPFParametersSourceCode }}

    bpf_ringbuf_submit(event, 0);

    return 0;
}

char __license[] SEC("license") = "GPL";
