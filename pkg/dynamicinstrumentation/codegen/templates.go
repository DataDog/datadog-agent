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
var sliceRegisterHeaderTemplateText = `
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);

outputOffset += 1;

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

var sliceStackHeaderTemplateText = `
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);

outputOffset += 1;

{{.SliceTypeHeaderText}}
`

var stringRegisterHeaderTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the string kind to output buffer
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);
outputOffset += 1;
`

var stringStackHeaderTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the string kind to output buffer
param_type = {{.Kind}};
bpf_probe_read(&event->output[outputOffset], sizeof(param_type), &param_type);
outputOffset += 1;
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
