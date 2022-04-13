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
	missing, err := VerifyKernelFuncs("./testdata/kallsyms.supported", requiredKernelFuncs)
	assert.Empty(t, missing)
	assert.Empty(t, err)

	missing, err = VerifyKernelFuncs("./testdata/kallsyms.unsupported", requiredKernelFuncs)
	assert.NotEmpty(t, missing)
	assert.Empty(t, err)

	missing, err = VerifyKernelFuncs("./testdata/kallsyms.empty", requiredKernelFuncs)
	assert.NotEmpty(t, missing)
	assert.Empty(t, err)

	_, err = VerifyKernelFuncs("./testdata/kallsyms.d_o_n_o_t_e_x_i_s_t", requiredKernelFuncs)
	assert.NotEmpty(t, err)
}
