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
bpf_probe_read_kernel(&event->output[context.output_offset], sizeof(param_type), &param_type);
param_size = {{.TotalSize}};
bpf_probe_read_kernel(&event->output[context.output_offset+sizeof(param_type)], sizeof(param_size), &param_size);
context.output_offset += sizeof(param_type) + sizeof(param_size);
`
var sliceRegisterHeaderTemplateText = `
// Name={{.Parameter.Name}} ID={{.Parameter.ID}} TotalSize={{.Parameter.TotalSize}} Kind={{.Parameter.Kind}}
// Write the slice kind to output buffer
param_type = {{.Parameter.Kind}};
bpf_probe_read_kernel(&event->output[context.output_offset], sizeof(param_type), &param_type);

context.output_offset += sizeof(param_type);

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
bpf_probe_read_kernel(&event->output[context.output_offset], sizeof(param_type), &param_type);

context.output_offset += sizeof(param_type);

{{.SliceTypeHeaderText}}
`

var stringHeaderTemplateText = `
// Name={{.Name}} ID={{.ID}} TotalSize={{.TotalSize}} Kind={{.Kind}}
// Write the string kind to output buffer
param_type = {{.Kind}};
bpf_probe_read_kernel(&event->output[context.output_offset], sizeof(param_type), &param_type);
context.output_offset += sizeof(param_type);
`
