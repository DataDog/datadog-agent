#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "kconfig.h"
#include <asm/ptrace.h>
#include "macros.h"
#include "base_event.h"
#include "event.h"
#include "maps.h"
#include "expressions.h"

SEC("uprobe/{{.GetBPFFuncName}}")
int {{.GetBPFFuncName}}(struct pt_regs *ctx)
{
    log_debug("{{.GetBPFFuncName}} probe in {{.ServiceName}} has triggered");

    // reserve space on ringbuffer
    event_t *event;
    event = bpf_ringbuf_reserve(&events, sizeof(event_t), 0);
    if (!event) {
        log_debug("No space available on ringbuffer, dropping event");
        return 0;
    }

    char* zero_string;
    __u32 key = 0;
    zero_string = bpf_map_lookup_elem(&zeroval, &key);
    if (!zero_string) {
        log_debug("couldn't lookup zero value in zeroval array map, dropping event for {{.GetBPFFuncName}}");
        bpf_ringbuf_discard(event, 0);
        return 0;
    }
    long err;
    err = bpf_probe_read_kernel(&event->base.probe_id, sizeof(event->base.probe_id), zero_string);
    if (err != 0) {
        log_debug("could not zero out probe id buffer");
    }
    err = bpf_probe_read_kernel(&event->base.program_counters, sizeof(event->base.program_counters), zero_string);
    if (err != 0) {
        log_debug("could not zero out program counter buffer");
    }
    err = bpf_probe_read_kernel(&event->output, sizeof(event->output), zero_string);
    if (err != 0) {
        log_debug("could not zero out output buffer");
    }
    err = bpf_probe_read_kernel(&event->base.probe_id, {{ .ID | len }}, "{{.ID}}");
    if (err != 0) {
        log_debug("could not write probe id to output");
    }
    err = bpf_probe_read_kernel(&event->base.param_indicies, sizeof(event->base.param_indicies), zero_string);
    if (err != 0) {
        log_debug("could not zero out param indicies");
    }

    // Get tid and tgid
    u64 pidtgid = bpf_get_current_pid_tgid();
    u32 tgid = pidtgid >> 32;
    event->base.pid = tgid;

    u64 uidgid = bpf_get_current_uid_gid();
    u32 uid = uidgid >> 32;
    event->base.uid = uid;

    // Collect stack trace
    __u64 currentPC = PT_REGS_IP(ctx);
    err = bpf_probe_read_kernel(&event->base.program_counters[0], sizeof(__u64), &currentPC);
    if (err != 0) {
        log_debug("could not collect first program counter");
    }

    __u64 bp = PT_REGS_FP(ctx);
    err = bpf_probe_read_user(&bp, sizeof(__u64), (void*)bp); // dereference bp to get current stack frame
    if (err != 0) {
        log_debug("could not retrieve base pointer for current stack frame");
    }

    __u64 ret_addr = PT_REGS_RET(ctx); // when bpf prog enters, the return address hasn't yet been written to the stack

    int i;
    int j;
    __u16 n;
    for (i = 1; i < STACK_DEPTH_LIMIT; i++)
    {
        if (bp == 0) {
            break;
        }
        err = bpf_probe_read_kernel(&event->base.program_counters[i], sizeof(__u64), &ret_addr);
        if (err != 0) {
            log_debug("error occurred while collecting program counter for stack trace (1)");
        }
        err = bpf_probe_read_user(&ret_addr, sizeof(__u64), (void*)(bp-8));
        if (err != 0) {
            log_debug("error occurred while collecting program counter for stack trace (2)");
        }
        err = bpf_probe_read_user(&bp, sizeof(__u64), (void*)bp);
        if (err != 0) {
            log_debug("error occurred while collecting program counter for stack trace (3)");
        }
    }

    // Collect parameters
    __u16 collectionMax = MAX_SLICE_LENGTH;
    __u8 param_type;
    __u16 param_size;
    __u16 slice_length;
    __u16 *collectionLimit;
    int chunk_size = 0;

    // Set up temporary storage array which is used by some location expressions
    // to have memory off the stack to work with
    __u64 *temp_storage = bpf_map_lookup_elem(&temp_storage_array, &key) ;
    if (!temp_storage) {
        log_debug("could not lookup temporary storage array");
        bpf_ringbuf_discard(event, 0);
        return 0;
    }

    u32 cpu = bpf_get_smp_processor_id();
    struct bpf_map *param_stack = (struct bpf_map*)bpf_map_lookup_elem(&param_stacks, &cpu);
    if (!param_stack) {
        log_debug("could not lookup param stack for cpu %d", cpu);
        bpf_ringbuf_discard(event, 0);
        return 0;
    }

    expression_context_t context = {
        .ctx = ctx,
        .event = event,
        .temp_storage = temp_storage,
        .zero_string = zero_string,
        .output_offset = 0,
        .stack_counter = 0,
        .param_stack = param_stack,
    };

    {{ .InstrumentationInfo.BPFParametersSourceCode }}

    bpf_ringbuf_submit(event, 0);


    // Drain the stack map for next invocation
    __u8 m = 0;
    __u64 placeholder;
    long pop_ret = 0;
    for (m = 0; m < context.stack_counter; m++) {
        pop_ret = bpf_map_pop_elem(context.param_stack, &placeholder);
        if (pop_ret != 0) {
            break;
        }
    }

    return 0;
}

char __license[] SEC("license") = "GPL";
