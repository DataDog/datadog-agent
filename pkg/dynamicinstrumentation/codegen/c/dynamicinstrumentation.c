#include "bpf_helpers.h"
#include "bpf_tracing.h"

#define MAX_STRING_SIZE {{ .InstrumentationInfo.InstrumentationOptions.StringMaxSize}}
#define PARAM_BUFFER_SIZE {{ .InstrumentationInfo.InstrumentationOptions.ArgumentsMaxSize}}
#define STACK_DEPTH_LIMIT 10
#define MAX_SLICE_SIZE 1800
#define MAX_SLICE_LENGTH 20

struct bpf_map_def SEC("maps") events = {
    .type        = BPF_MAP_TYPE_RINGBUF,
    .max_entries = 1<<24,
};

struct bpf_map_def SEC("maps") zeroval = {
    .type        = BPF_MAP_TYPE_ARRAY,
    .key_size    = sizeof(u32),
    .value_size  = sizeof(char[PARAM_BUFFER_SIZE]),
    .max_entries = 1,
};

// NOTE: Be careful when adding fields, alignment should always be to 8 bytes
// Parsing logic in user space must be updated for field offsets each time
// new fields are added
struct event {
    char probe_id[304];
    __u32 pid;
    __u32 uid;
    __u64 program_counters[10];
    char output[PARAM_BUFFER_SIZE];
};

SEC("uprobe/{{.GetBPFFuncName}}")
int {{.GetBPFFuncName}}(struct pt_regs *ctx)
{
    bpf_printk("{{.GetBPFFuncName}} probe in {{.ServiceName}} has triggered");

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

    bpf_probe_read(&event->probe_id, sizeof(event->probe_id), zero_string);
    bpf_probe_read(&event->program_counters, sizeof(event->program_counters), zero_string);
    bpf_probe_read(&event->output, sizeof(event->output), zero_string);

    bpf_probe_read(&event->probe_id, {{ .ID | len }}, "{{.ID}}");

    // Get tid and tgid
    u64 pidtgid = bpf_get_current_pid_tgid();
    u32 tgid = pidtgid >> 32;
    event->pid = tgid;

    u64 uidgid = bpf_get_current_uid_gid();
    u32 uid = uidgid >> 32;
    event->uid = uid;

    // Collect stack trace
    __u64 currentPC = ctx->pc;
    bpf_probe_read(&event->program_counters[0], sizeof(__u64), &currentPC);

    __u64 bp = ctx->regs[29];
    bpf_probe_read(&bp, sizeof(__u64), (void*)bp); // dereference bp to get current stack frame
    __u64 ret_addr = ctx->regs[30]; // when bpf prog enters, the return address hasn't yet been written to the stack

    int i;
    for (i = 1; i < STACK_DEPTH_LIMIT; i++)
    {
        if (bp == 0) {
            break;
        }
        bpf_probe_read(&event->program_counters[i], sizeof(__u64), &ret_addr);
        bpf_probe_read(&ret_addr, sizeof(__u64), (void*)(bp-8));
        bpf_probe_read(&bp, sizeof(__u64), (void*)bp);
    }

    // Collect parameters
    __u8 param_type;
    __u16 param_size;
    __u16 slice_length;

    int outputOffset = 0;

    {{ .InstrumentationInfo.BPFParametersSourceCode }}

    bpf_ringbuf_submit(event, 0);

    return 0;
}

char __license[] SEC("license") = "GPL";
