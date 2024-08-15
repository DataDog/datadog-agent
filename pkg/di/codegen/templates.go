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
#define MAX_SLICE_SIZE 1800
#define MAX_SLICE_LENGTH 20

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
    __u16 slice_length;

    int outputOffset = 0;

    {{ .PopulatedParameterText }}

    bpf_ringbuf_submit(event, 0);

    return 0;
}

char __license[] SEC("license") = "GPL";
`

var forcedVerifierErrorTemplate = `
int illegalDereference = *(*(*ctx->regs[0]));
`

var headerTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the kind and size to output buffer
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);
param_size = {{.TotalSize}};
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);
outputOffset += 3;
`

// The length of slices aren't known until parsing, so they require
// special headers to read in the length dynamically
var sliceRegisterHeaderTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);
// Read slice length and write it to output buffer
bpf_probe_read(&param_size, 8, &ctx->regs[{{.Parameter.Location.Register}}+1]);
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);
outputOffset += 3;

slice_length = param_size;

for (i = 0; i < MAX_SLICE_LENGTH; i++) {
    if (i >= slice_length) {
        break;
    }
    {{.SliceTypeHeaderText}}
}
`

// The length of slices aren't known until parsing, so they require
// special headers to read in the length dynamically
var sliceStackHeaderTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);
// Read slice length and write it to output buffer
bpf_probe_read(&param_size, 8, &ctx->regs[29]+{{.Location.StackOffset}}+8]);
bpf_probe_read(&event->output[outputOffset+1], 2, &param_size);
outputOffset += 3;

slice_length = param_size;
if (slice_length > MAX_SLICE_LENGTH) {
    slice_length = MAX_SLICE_LENGTH;
}

for (i = 0; i < slice_length; i++) {
    {{.SliceTypeHeaderText}}
}
`

// The length of strings aren't known until parsing, so they require
// special headers to read in the length dynamically
var stringRegisterHeaderTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the string kind to output buffer
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);

// Read string length and write it to output buffer
bpf_probe_read(&param_size, 8, &ctx->regs[{{.Location.Register}}+1]);

// Limit string length
__u16 string_size_{{.ID}} = param_size;
if (string_size_{{.ID}} > MAX_STRING_SIZE) {
    string_size_{{.ID}} = MAX_STRING_SIZE;
}
bpf_probe_read(&event->output[outputOffset+1], 2, &string_size_{{.ID}});
outputOffset += 3;
`

// The length of strings aren't known until parsing, so they require
// special headers to read in the length dynamically
var stringStackHeaderTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the string kind to output buffer
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], 1, &param_type);
// Read string length and write it to output buffer
bpf_probe_read(&param_size, 8, (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8));
// Limit string length
__u16 string_size_{{.ID}} = param_size;
if (string_size_{{.ID}} > MAX_STRING_SIZE) {
    string_size_{{.ID}} = MAX_STRING_SIZE;
}
bpf_probe_read(&event->output[outputOffset+1], 2, &string_size_{{.ID}});
outputOffset += 3;
`

var sliceRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read contents of slice
bpf_probe_read(&event->output[outputOffset], MAX_SLICE_SIZE, (void*)ctx->regs[{{.Location.Register}}]);
outputOffset += MAX_SLICE_SIZE;
`

var sliceStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read contents of slice
bpf_probe_read(&event->output[outputOffset], MAX_SLICE_SIZE, (void*)(ctx->regs[29]+{{.Location.StackOffset}});
outputOffset += MAX_SLICE_SIZE;`

var stringRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read string length and write it to output buffer
bpf_probe_read(&param_size, 8, &ctx->regs[{{.Location.Register}}+1]);

__u16 string_size_read_{{.ID}} = param_size;
if (string_size_read_{{.ID}} > MAX_STRING_SIZE) {
    string_size_read_{{.ID}} = MAX_STRING_SIZE;
}

// Read contents of string
bpf_probe_read(&event->output[outputOffset], string_size_read_{{.ID}}, (void*)ctx->regs[{{.Location.Register}}]);
outputOffset += string_size_read_{{.ID}};
`

var stringStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read string length and write it to output buffer
bpf_probe_read(&param_size, 8, (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8));
// Limit string length
__u16 string_size_read_{{.ID}} = param_size;
if (string_size_read_{{.ID}} > MAX_STRING_SIZE) {
    string_size_read_{{.ID}} = MAX_STRING_SIZE;
}
// Read contents of string
bpf_probe_read(&ret_addr, sizeof(__u64), (void*)(ctx->regs[29]+{{.Location.StackOffset}}));
bpf_probe_read(&event->output[outputOffset], string_size_read_{{.ID}}, (void*)(ret_addr));
outputOffset += string_size_read_{{.ID}};
`

var pointerRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read the pointer value (address of underlying value)
void *ptrTo{{.ID}};
bpf_probe_read(&ptrTo{{.ID}}, 8, &ctx->regs[{{.Location.Register}}]);

// Write the underlying value to output
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, ptrTo{{.ID}}+{{.Location.PointerOffset}});
outputOffset += {{.TotalSize}};

// Write the pointer address to output
ptrTo{{.ID}} += {{.Location.PointerOffset}};
bpf_probe_read(&event->output[outputOffset], 8, &ptrTo{{.ID}});
`

var pointerStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read the pointer value (address of underlying value)
void *ptrTo{{.ID}};
bpf_probe_read(&ptrTo{{.Name}}, 8, (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8));

// Write the underlying value to output
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, ptrTo{{.ID}}+{{.Location.PointerOffset}});
outputOffset += {{.TotalSize}};

// Write the pointer address to output
ptrTo{{.ID}} += {{.Location.PointerOffset}};
bpf_probe_read(&event->output[outputOffset], 8, &ptrTo{{.ID}});
`

var normalValueRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, &ctx->regs[{{.Location.Register}}]);
outputOffset += {{.TotalSize}};
`

var normalValueStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read value for {{.Name}}
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, (char*)((ctx->regs[29])+{{.Location.StackOffset}}));
outputOffset += {{.TotalSize}};
`

// Unsupported types just get a single `255` value to signify as a placeholder
// that an unsupported type goes here. Size is where we keep the actual type.
var unsupportedTypeTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// No capture, unsupported type
`

var cutForFieldLimitTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// No capture, cut for field limit
`
