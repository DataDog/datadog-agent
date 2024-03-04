// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

func TestEbpfConntrackerLoadTriggersOffsetGuessing(t *testing.T) {
	kv, err := ebpfkernel.NewKernelVersion()
	require.NoError(t, err)

	if kv.Code < ebpfkernel.Kernel4_14 && !kv.IsRH7Kernel() {
		t.Skip("Skipping test on unsupported kernel")
	}

	offsetguess.TracerOffsets.Reset()

	cfg := testConfig()
	cfg.EnableRuntimeCompiler = false
	conntracker, err := NewEBPFConntracker(cfg)
	assert.NoError(t, err)
	require.NotNil(t, conntracker)
	t.Cleanup(conntracker.Close)

	offsets, err := offsetguess.TracerOffsets.Offsets(cfg)
	require.NoError(t, err)
	require.NotEmpty(t, offsets)
}

func TestEbpfConntrackerSkipsLoadOnOlderKernels(t *testing.T) {
	kv, err := ebpfkernel.NewKernelVersion()
	require.NoError(t, err)

	if kv.Code >= ebpfkernel.Kernel4_14 || kv.IsRH7Kernel() {
		t.Skip("This test should only run on pre-4.14 kernels or kernels with backported eBPF support")
	}

	offsetguess.TracerOffsets.Reset()

	cfg := testConfig()
	cfg.EnableRuntimeCompiler = false
	conntracker, err := NewEBPFConntracker(cfg)
	assert.Error(t, err)
	assert.Equal(t, "ebpf conntracker requires kernel version 4.14 or higher or a RHEL kernel with backported eBPF support", err.Error())
	require.Nil(t, conntracker)
}
