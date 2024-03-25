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
)

func ebpfConntrackerSupportedOnKernelT(t *testing.T) bool {
	supported, err := ebpfConntrackerSupportedOnKernel()
	require.NoError(t, err)
	return supported
}

func skipEbpfConntrackerTestOnUnsupportedKernel(t *testing.T) {
	if !ebpfConntrackerSupportedOnKernelT(t) {
		t.Skip("Skipping ebpf conntracker related test on unsupported kernel")
	}
}

func TestEbpfConntrackerLoadTriggersOffsetGuessing(t *testing.T) {
	skipEbpfConntrackerTestOnUnsupportedKernel(t)

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
	if ebpfConntrackerSupportedOnKernelT(t) {
		t.Skip("This test should only run on pre-4.14 kernels without backported eBPF support, like RHEL/CentOS")
	}

	offsetguess.TracerOffsets.Reset()

	cfg := testConfig()
	cfg.EnableRuntimeCompiler = false
	conntracker, err := NewEBPFConntracker(cfg)
	assert.Error(t, err)
	assert.Equal(t,
		"could not load prebuilt ebpf conntracker: ebpf conntracker requires kernel version 4.14 or higher or a RHEL kernel with backported eBPF support",
		err.Error())
	require.Nil(t, conntracker)
}
