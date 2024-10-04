// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

var headerTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the kind and size to output buffer
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);
param_size = {{.TotalSize}};
bpf_probe_read(&event->output[outputOffset+1], sizeof(param_size), &param_size);
outputOffset += 3;
`

// The length and type of slices aren't known until parsing, so they require
// special headers to read in the length dynamically
var sliceRegisterHeaderTemplateText = `
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);

{{.SliceLengthText}}

bpf_probe_read(&event->output[outputOffset+1], sizeof(param_size), &param_size);
outputOffset += 3;

__u16 indexSlice{{.Parameter.ID}};
slice_length = param_size;
if (slice_length > MAX_SLICE_LENGTH) {
    slice_length = MAX_SLICE_LENGTH;
}

for (indexSlice{{.Parameter.ID}} = 0; indexSlice{{.Parameter.ID}} < MAX_SLICE_LENGTH; indexSlice{{.Parameter.ID}}++) {
    if (indexSlice{{.Parameter.ID}} >= slice_length) {
        break;
    }
    {{.SliceTypeHeaderText}}
}
`

var sliceLengthRegisterTemplateText = `
bpf_probe_read(&param_size, sizeof(param_size), &ctx->DI_REGISTER_{{.Location.Register}});
`

// The length and type of slices aren't known until parsing, so they require
// special headers to read in the length dynamically
var sliceStackHeaderTemplateText = `
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);

{{.SliceLengthText}}

bpf_probe_read(&event->output[outputOffset+1], sizeof(param_size), &param_size);
outputOffset += 3;

__u16 indexSlice{{.Parameter.ID}};
slice_length = param_size;
if (slice_length > MAX_SLICE_LENGTH) {
    slice_length = MAX_SLICE_LENGTH;
}

for (indexSlice{{.Parameter.ID}} = 0; indexSlice{{.Parameter.ID}} < MAX_SLICE_LENGTH; indexSlice{{.Parameter.ID}}++) {
    if (indexSlice{{.Parameter.ID}} >= slice_length) {
        break;
    }
    {{.SliceTypeHeaderText}}
}
`

var sliceLengthStackTemplateText = `
bpf_probe_read(&param_size, sizeof(param_size), &ctx->__PT_FP_REG+{{.Parameter.Location.StackOffset}}]);
`

// The length of strings aren't known until parsing, so they require
// special headers to read in the length dynamically
var stringRegisterHeaderTemplateText = `
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the string kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);

// Read string length and write it to output buffer

{{.StringLengthText}}

// Limit string length
__u16 string_size_{{.Parameter.ID}} = param_size;
if (string_size_{{.Parameter.ID}} > MAX_STRING_SIZE) {
    string_size_{{.Parameter.ID}} = MAX_STRING_SIZE;
}
bpf_probe_read(&event->output[outputOffset+1], sizeof(string_size_{{.Parameter.ID}}), &string_size_{{.Parameter.ID}});
outputOffset += 3;
`

var stringLengthRegisterTemplateText = `
bpf_probe_read(&param_size, sizeof(param_size), &ctx->DI_REGISTER_{{.Location.Register}});
`

// The length of strings aren't known until parsing, so they require
// special headers to read in the length dynamically
var stringStackHeaderTemplateText = `
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the string kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);

// Read string length and write it to output buffer
{{.StringLengthText}}

// Limit string length
__u16 string_size_{{.Parameter.ID}} = param_size;
if (string_size_{{.Parameter.ID}} > MAX_STRING_SIZE) {
    string_size_{{.Parameter.ID}} = MAX_STRING_SIZE;
}
bpf_probe_read(&event->output[outputOffset+1], sizeof(string_size_{{.Parameter.ID}}), &string_size_{{.Parameter.ID}});
outputOffset += 3;
`

var stringLengthStackTemplateText = `
bpf_probe_read(&param_size, sizeof(param_size), (char*)((ctx->__PT_FP_REG)+{{.Location.StackOffset}}));
`

var sliceRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read contents of slice
bpf_probe_read(&event->output[outputOffset], MAX_SLICE_SIZE, (void*)ctx->DI_REGISTER_{{.Location.Register}});
outputOffset += MAX_SLICE_SIZE;
`

var sliceStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read contents of slice
bpf_probe_read(&event->output[outputOffset], MAX_SLICE_SIZE, (void*)(ctx->__PT_FP_REG+{{.Location.StackOffset}});
outputOffset += MAX_SLICE_SIZE;`

var stringRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read string length and write it to output buffer

// We limit string length variable again in case the verifier forgot about it (which often happens)
__u16 string_size_{{.ID}}_new;
string_size_{{.ID}}_new = string_size_{{.ID}};
if (string_size_{{.ID}}_new > MAX_STRING_SIZE) {
    string_size_{{.ID}}_new = MAX_STRING_SIZE;
}

// Read contents of string
bpf_probe_read(&event->output[outputOffset], string_size_{{.ID}}_new, (void*)ctx->DI_REGISTER_{{.Location.Register}});
outputOffset += string_size_{{.ID}}_new;
`

var stringStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}

// We limit string length variable again in case the verifier forgot about it (which often happens)
__u16 string_size_{{.ID}}_new;
string_size_{{.ID}}_new = string_size_{{.ID}};
if (string_size_{{.ID}}_new > MAX_STRING_SIZE) {
    string_size_{{.ID}}_new = MAX_STRING_SIZE;
}
// Read contents of string
bpf_probe_read(&ret_addr, sizeof(__u64), (void*)(ctx->__PT_FP_REG+{{.Location.StackOffset}}));
bpf_probe_read(&event->output[outputOffset], string_size_{{.ID}}_new, (void*)(ret_addr));
outputOffset += string_size_{{.ID}}_new;
`

var pointerRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read the pointer value (address of underlying value)
void *ptrTo{{.ID}};
bpf_probe_read(&ptrTo{{.ID}}, sizeof(ptrTo{{.ID}}), &ctx->DI_REGISTER_{{.Location.Register}});

// Write the underlying value to output
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, ptrTo{{.ID}}+{{.Location.PointerOffset}});
outputOffset += {{.TotalSize}};

// Write the pointer address to output
ptrTo{{.ID}} += {{.Location.PointerOffset}};
bpf_probe_read(&event->output[outputOffset], sizeof(ptrTo{{.ID}}), &ptrTo{{.ID}});
`

var pointerStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read the pointer value (address of underlying value)
void *ptrTo{{.ID}};
bpf_probe_read(&ptrTo{{.ID}}, sizeof(ptrTo{{.ID}}), (char*)((ctx->__PT_FP_REG)+{{.Location.StackOffset}}+8));

// Write the underlying value to output
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, ptrTo{{.ID}}+{{.Location.PointerOffset}});
outputOffset += {{.TotalSize}};

// Write the pointer address to output
ptrTo{{.ID}} += {{.Location.PointerOffset}};
bpf_probe_read(&event->output[outputOffset], sizeof(ptrTo{{.ID}}), &ptrTo{{.ID}});
`

var normalValueRegisterTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, &ctx->DI_REGISTER_{{.Location.Register}});
outputOffset += {{.TotalSize}};
`

var normalValueStackTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Read value for {{.Name}}
bpf_probe_read(&event->output[outputOffset], {{.TotalSize}}, (char*)((ctx->__PT_FP_REG)+{{.Location.StackOffset}}));
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
