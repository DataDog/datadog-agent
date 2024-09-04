// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

var forcedVerifierErrorTemplate = `
int illegalDereference = *(*(*ctx->regs[0]));
`

var headerTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the kind and size to output buffer
param_type = {{.Kind}};
bpf_probe_read_kernel(&event->output[outputOffset], 1, &param_type);
param_size = {{.TotalSize}};
bpf_probe_read_kernel(&event->output[outputOffset+1], 2, &param_size);
outputOffset += 3;
`

// The length of slices aren't known until parsing, so they require
// special headers to read in the length dynamically
var sliceRegisterHeaderTemplateText = `
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read_kernel(&event->output[outputOffset], 1, &param_type);
// Read slice length and write it to output buffer
bpf_probe_read_user(&param_size, 8, &ctx->regs[{{.Parameter.Location.Register}}+1]);
bpf_probe_read_kernel(&event->output[outputOffset+1], 2, &param_size);
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
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read_kernel(&event->output[outputOffset], 1, &param_type);
// Read slice length and write it to output buffer
bpf_probe_read_user(&param_size, 8, &ctx->regs[29]+{{.Parameter.Location.StackOffset}}+8]);
bpf_probe_read_kernel(&event->output[outputOffset+1], 2, &param_size);
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
bpf_probe_read_kernel(&event->output[outputOffset], 1, &param_type);

// Read string length and write it to output buffer
bpf_probe_read_user(&param_size, 8, &ctx->regs[{{.Location.Register}}+1]);

// Limit string length
__u16 string_size_{{.ID}} = param_size;
if (string_size_{{.ID}} > MAX_STRING_SIZE) {
    string_size_{{.ID}} = MAX_STRING_SIZE;
}
bpf_probe_read_kernel(&event->output[outputOffset+1], 2, &string_size_{{.ID}});
outputOffset += 3;
`

// The length of strings aren't known until parsing, so they require
// special headers to read in the length dynamically
var stringStackHeaderTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the string kind to output buffer
param_type = {{.Kind}};
bpf_probe_read_kernel(&event->output[outputOffset], 1, &param_type);
// Read string length and write it to output buffer
bpf_probe_read_user(&param_size, 8, (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8));
// Limit string length
__u16 string_size_{{.ID}} = param_size;
if (string_size_{{.ID}} > MAX_STRING_SIZE) {
    string_size_{{.ID}} = MAX_STRING_SIZE;
}
bpf_probe_read_kernel(&event->output[outputOffset+1], 2, &string_size_{{.ID}});
outputOffset += 3;
`

var sliceRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read contents of slice
bpf_probe_read_user(&event->output[outputOffset], MAX_SLICE_SIZE, (void*)ctx->regs[{{.Location.Register}}]);
outputOffset += MAX_SLICE_SIZE;
`

var sliceStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read contents of slice
bpf_probe_read_user(&event->output[outputOffset], MAX_SLICE_SIZE, (void*)(ctx->regs[29]+{{.Location.StackOffset}});
outputOffset += MAX_SLICE_SIZE;`

var stringRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read string length and write it to output buffer
bpf_probe_read_user(&param_size, 8, &ctx->regs[{{.Location.Register}}+1]);

__u16 string_size_read_{{.ID}} = param_size;
if (string_size_read_{{.ID}} > MAX_STRING_SIZE) {
    string_size_read_{{.ID}} = MAX_STRING_SIZE;
}

// Read contents of string
bpf_probe_read_user(&event->output[outputOffset], string_size_read_{{.ID}}, (void*)ctx->regs[{{.Location.Register}}]);
outputOffset += string_size_read_{{.ID}};
`

var stringStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read string length and write it to output buffer
bpf_probe_read_user(&param_size, 8, (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8));
// Limit string length
__u16 string_size_read_{{.ID}} = param_size;
if (string_size_read_{{.ID}} > MAX_STRING_SIZE) {
    string_size_read_{{.ID}} = MAX_STRING_SIZE;
}
// Read contents of string
bpf_probe_read_user(&ret_addr, sizeof(__u64), (void*)(ctx->regs[29]+{{.Location.StackOffset}}));
bpf_probe_read_user(&event->output[outputOffset], string_size_read_{{.ID}}, (void*)(ret_addr));
outputOffset += string_size_read_{{.ID}};
`

var pointerRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read the pointer value (address of underlying value)
void *ptrTo{{.ID}};
bpf_probe_read_user(&ptrTo{{.ID}}, 8, &ctx->regs[{{.Location.Register}}]);

// Write the underlying value to output
bpf_probe_read_user(&event->output[outputOffset], {{.TotalSize}}, ptrTo{{.ID}}+{{.Location.PointerOffset}});
outputOffset += {{.TotalSize}};

// Write the pointer address to output
ptrTo{{.ID}} += {{.Location.PointerOffset}};
bpf_probe_read_user(&event->output[outputOffset], 8, &ptrTo{{.ID}});
`

var pointerStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read the pointer value (address of underlying value)
void *ptrTo{{.ID}};
bpf_probe_read_user(&ptrTo{{.Name}}, 8, (char*)((ctx->regs[29])+{{.Location.StackOffset}}+8));

// Write the underlying value to output
bpf_probe_read_user(&event->output[outputOffset], {{.TotalSize}}, ptrTo{{.ID}}+{{.Location.PointerOffset}});
outputOffset += {{.TotalSize}};

// Write the pointer address to output
ptrTo{{.ID}} += {{.Location.PointerOffset}};
bpf_probe_read_user(&event->output[outputOffset], 8, &ptrTo{{.ID}});
`

var normalValueRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
bpf_probe_read_user(&event->output[outputOffset], {{.TotalSize}}, &ctx->regs[{{.Location.Register}}]);
outputOffset += {{.TotalSize}};
`

var normalValueStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read value for {{.Name}}
bpf_probe_read_user(&event->output[outputOffset], {{.TotalSize}}, (char*)((ctx->regs[29])+{{.Location.StackOffset}}));
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
