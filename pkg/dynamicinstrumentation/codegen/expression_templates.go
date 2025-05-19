// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

var readRegisterTemplateText = `
// Arg1 = register
// Arg2 = size of element
read_register(&context, {{.Arg1}}, {{.Arg2}});
`

var readStackTemplateText = `
// Arg1 = stack offset
// Arg2 = size of element
read_stack(&context, {{.Arg1}}, {{.Arg2}});
`

var readRegisterValueToOutputTemplateText = `
// Arg1 = register
// Arg2 = size of element
read_register_value_to_output(&context, {{.Arg1}}, {{.Arg2}});
`

var readStackValueToOutputTemplateText = `
// Arg1 = stack offset
// Arg2 = size of element
read_stack_value_to_output(&context, {{.Arg1}}, {{.Arg2}});
`

var popTemplateText = `
// Arg1 = number of elements (u64) to pop
// Arg2 = size of each element
pop(&context, {{.Arg1}}, {{.Arg2}});
`

var dereferenceTemplateText = `
// Arg1 = size in bytes of value we're reading from the 8 byte address at the top of the stack
dereference(&context, {{.Arg1}});
`

var dereferenceToOutputTemplateText = `
// Arg1 = size in bytes of value we're reading from the 8 byte address at the top of the stack
dereference_to_output(&context, {{.Arg1}});
`

var dereferenceLargeTemplateText = `
// Arg1 = size in bytes of value we're reading from the 8 byte address at the top of the stack
// Arg2 = number of chunks (should be ({{.Arg1}} + 7) / 8)
dereference_large(&context, {{.Arg1}}, {{.Arg2}});
`

var dereferenceLargeToOutputTemplateText = `
// Arg1 = size in bytes of value we're reading from the 8 byte address at the top of the stack
dereference_large_to_output(&context, {{.Arg1}});
`

var applyOffsetTemplateText = `
// Arg1 = uint value (offset) we're adding to the 8-byte address on top of the stack
apply_offset(&context, {{.Arg1}});
`

var dereferenceDynamicTemplateText = `
// Arg1 = maximum limit on bytes read
// Arg2 = number of chunks (should be (max + 7)/8)
// Arg3 = size of each element
dereference_dynamic(&context, {{.Arg1}}, {{.Arg2}}, {{.Arg3}});
`

var dereferenceDynamicToOutputTemplateText = `
// Arg1 = maximum limit on bytes read
dereference_dynamic_to_output(&context, {{.Arg1}});
`

var readStringToOutputTemplateText = `
// Arg1 = maximum limit on string length
read_str_to_output(&context, {{.Arg1}});
`

var copyTemplateText = `
copy(&context);
`

var setLimitEntryText = `
// Arg1 = Maximum limit
set_limit_entry(&context, {{.Arg1}}, "{{.CollectionIdentifier}}");
`

var jumpIfGreaterThanLimitText = `
collectionLimit = bpf_map_lookup_elem(&collection_limits, "{{.CollectionIdentifier}}");
if (!collectionLimit) {
    log_debug("couldn't find collection limit for {{.CollectionIdentifier}}");
    collectionLimit = &collectionMax;
}
if ({{.Arg1}} == *collectionLimit) {
    log_debug("collection limit for {{.CollectionIdentifier}} exceeded: %d", *collectionLimit);
    goto {{.Label}};
}
`

var labelTemplateText = `
{{.Label}}:
`

var commentText = `
// {{.Label}}
`

var printStatementText = `
log_debug("{{.Label}}", "{{.CollectionIdentifier}}");
`

var setParameterIndexText = `
bpf_printk("Setting param index %d to %d", {{.Arg1}}, context.output_offset);
event->base.param_indicies[{{.Arg1}}] = context.output_offset;
`

// This causes a compiler error which is used in testing
var compilerErrorText = `
!@#$%^
`

// This causes a verifier error which is used in testing
var verifierErrorText = `
for (int i=0; i==0;) {
    i++;
    i--;
}
`
