// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var requiredKernelFuncs = []string{
	// Maps (3.18)
	"bpf_map_lookup_elem",
	"bpf_map_update_elem",
	"bpf_map_delete_elem",
	// bpf_probe_read intentionally omitted since it was renamed in kernel 5.5
	// Perf events (4.4)
	"bpf_perf_event_output",
	"bpf_perf_event_read",
}

func TestVerifyKernelFuncs(t *testing.T) {
	fc := newExistCache("./testdata/kallsyms.supported")
	missing, err := fc.verifyKernelFuncs(requiredKernelFuncs)
	assert.Empty(t, missing)
	assert.Empty(t, err)

	fc = newExistCache("./testdata/kallsyms.unsupported")
	missing, err = fc.verifyKernelFuncs(requiredKernelFuncs)
	assert.Len(t, missing, len(requiredKernelFuncs))
	assert.Empty(t, err)

	fc = newExistCache("./testdata/kallsyms.empty")
	missing, err = fc.verifyKernelFuncs(requiredKernelFuncs)
	assert.Len(t, missing, len(requiredKernelFuncs))
	assert.Empty(t, err)

	fc = newExistCache("./testdata/kallsyms.d_o_n_o_t_e_x_i_s_t")
	_, err = fc.verifyKernelFuncs(requiredKernelFuncs)
	assert.NotEmpty(t, err)
}

func BenchmarkVerifyKernelFuncs(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fc := newExistCache("./testdata/kallsyms.supported")
		fc.verifyKernelFuncs(requiredKernelFuncs)
	}
}
