package codegen

var programTemplateText = `
#include "vmlinux_arm64.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "go_runtime_types.bpf.h"
#include "ringbuffer.h"

#define MAX_STRING_SIZE {{.Probe.InstrumentationInfo.InstrumentationOptions.StringMaxSize}}
#define PARAM_BUFFER_SIZE {{.Probe.InstrumentationInfo.InstrumentationOptions.ArgumentsMaxSize}}
#define STACK_DEPTH_LIMIT 10

struct bpf_map_def SEC("maps") zeroval = {
    .type        = BPF_MAP_TYPE_ARRAY,
    .key_size    = sizeof(u32),
    .value_size  = sizeof(char[PARAM_BUFFER_SIZE]),
    .max_entries = 1,
};

// Hash map where k = TID, v = goroutine ID
// used to retrieve goroutine ids for triggered
// events by setting the goroutine ID from
// instrumenting runtime.execute
struct bpf_map_def SEC("maps") tgs = {
    .type        = BPF_MAP_TYPE_HASH,
    .key_size    = sizeof(u32),
    .value_size  = sizeof(u64),
    .max_entries = 1<<16,
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

SEC("uprobe/{{.Probe.GetBPFFuncName}}")
int {{.Probe.GetBPFFuncName}}(struct pt_regs *ctx)
{
    bpf_printk("{{.Probe.GetBPFFuncName}} probe in {{.Probe.ServiceName}} has triggered");

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
        bpf_printk("couldn't lookup zero value in zeroval array map, dropping event for {{.Probe.GetBPFFuncName}}");
        bpf_ringbuf_discard(event, 0);
        return 0;
    }

    bpf_probe_read(&event->probe_id, sizeof(event->probe_id), zero_string);
    bpf_probe_read(&event->program_counters, sizeof(event->program_counters), zero_string);
    bpf_probe_read(&event->output, sizeof(event->output), zero_string);

    bpf_probe_read(&event->probe_id, {{ .Probe.ID | len }}, "{{.Probe.ID}}");

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

    int outputOffset = 0;

    {{ .PopulatedParameterText }}
    bpf_ringbuf_submit(event, 0);

    return 0;
}

char __license[] SEC("license") = "GPL";
`

var sliceRegisterTemplateText = ``
var slicePointerRegisterTemplateText = ``

var sliceStackTemplateText = ``
var slicePointerStackTemplateText = ``

var stringRegisterTemplateText = `
// Read the type
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

// Read string length
bpf_probe_read(&param_size, 8, &ctx->regs[{{.Location.Register}}+1]);
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);
outputOffset += 3;

// Limit string length
int string_size_{{.ID}} = param_size;
if (string_size_{{.ID}} > MAX_STRING_SIZE) {
    string_size_{{.ID}} = MAX_STRING_SIZE;
}

// Read Actual String
bpf_probe_read(&event->output[outputOffset], string_size_{{.ID}}, (void*)ctx->regs[{{.Location.Register}}]);
outputOffset += string_size_{{.ID}};
`

var stringPointerRegisterTemplateText = `
// Read the type
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

// Get string address
void* locationOfStringStructure{{.ID}} = (void*)ctx->regs[{{.Location.Register}}];
char* char_array_{{.ID}};

// Read string length and address of array
bpf_probe_read(&param_size, 8, locationOfStringStructure{{.ID}}+8);
bpf_probe_read(&char_array_{{.ID}}, 8, locationOfStringStructure{{.ID}});

bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);
outputOffset += 3;

// Limit string length
int string_size_{{.ID}} = param_size;
if (string_size_{{.ID}} > MAX_STRING_SIZE) {
    string_size_{{.ID}} = MAX_STRING_SIZE;
}

// Read actual string
bpf_probe_read(&event->output[outputOffset], string_size_{{.ID}}, (char*)char_array_{{.ID}});
outputOffset += string_size_{{.ID}};
`

var pointerRegisterTemplateText = `
void *ptrTo{{.ID}};
bpf_probe_read(&ptrTo{{.ID}}, 8, &ctx->regs[{{.Location.Register}}]);

param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

param_size = {{.TotalSize}};
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);

outputOffset += 3;
{{ if and (ne .Kind 25) (ne .Kind 24) (ne .Kind 23) (ne .Kind 17)}}
// Read the actual value of this type (Not a type header)
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, ptrTo{{.ID}}+{{.Location.PointerOffset}});
outputOffset += {{.TotalSize}};
{{ end }}
`

var normalValueRegisterTemplateText = `
// Read the type
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

// Read the length
param_size = {{.TotalSize}};
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);

outputOffset += 3;

{{ if and (ne .Kind 25) (ne .Kind 24) (ne .Kind 23) (ne .Kind 17)}}
// Actual value (if not a type header)
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, &ctx->regs[{{.Location.Register}}]);
outputOffset += {{.TotalSize}};
{{ end }}
`

var stringStackTemplateText = `
// Read the type
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

bpf_probe_read(&param_size, 8, (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8));
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);
outputOffset += 3;

int string_size_{{.Name}} = param_size;
if (string_size_{{.Name}} > MAX_STRING_SIZE) {
    string_size_{{.Name}} = MAX_STRING_SIZE;
}
bpf_probe_read(&ret_addr, sizeof(__u64), (void*)(ctx->regs[29]+{{.Location.StackOffset}}));
bpf_probe_read(&event->output[outputOffset], string_size_{{.Name}}, (void*)(ret_addr));
outputOffset += string_size_{{.Name}};
`

var stringPointerStackTemplateText = `
// Read the type
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

void* locationOfStringStructure{{.ID}} = (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8);
char* char_array_{{.ID}};

bpf_probe_read(&param_size, 8, locationOfStringStructure{{.ID}}+8);
bpf_probe_read(&char_array_{{.ID}}, 8, locationOfStringStructure{{.ID}});

bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);
outputOffset += 3;

int string_size_{{.ID}} = param_size;
if (string_size_{{.ID}} > MAX_STRING_SIZE) {
    string_size_{{.ID}} = MAX_STRING_SIZE;
}

bpf_probe_read(&event->output[outputOffset], string_size_{{.ID}}, (char*)char_array_{{.ID}});
outputOffset += string_size_{{.ID}};
`

var pointerStackTemplateText = `
// Read the type
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

void *ptrTo{{.Name}};
bpf_probe_read(&ptrTo{{.Name}}, 8, (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8));

param_size = {{.TotalSize}};
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);

outputOffset += 3;
{{ if and (ne .Kind 25) (ne .Kind 24) (ne .Kind 23) (ne .Kind 17)}}
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, ptrTo{{.Name}}+{{.Location.PointerOffset}});
outputOffset += {{.TotalSize}};
{{ end }}
`

var normalValueStackTemplateText = `
// Read the type
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

// Length
param_size = {{.TotalSize}};
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);

outputOffset += 3;
{{ if and (ne .Kind 25) (ne .Kind 24) (ne .Kind 23) (ne .Kind 17)}}
// Actual value (if not a type header)
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, (char*)((ctx->regs[29])+{{.Location.StackOffset}}));
outputOffset += {{.TotalSize}};
{{ end }}
`
