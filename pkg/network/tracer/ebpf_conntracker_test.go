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

func ebpfPrebuiltConntrackerSupportedOnKernelT(t *testing.T) bool {
	supported, err := ebpfPrebuiltConntrackerSupportedOnKernel()
	require.NoError(t, err)
	return supported
}

func ebpfCOREConntrackerSupportedOnKernelT(t *testing.T) bool {
	supported, err := ebpfCOREConntrackerSupportedOnKernel()
	require.NoError(t, err)
	return supported
}

func skipPrebuiltEbpfConntrackerTestOnUnsupportedKernel(t *testing.T) {
	if !ebpfPrebuiltConntrackerSupportedOnKernelT(t) {
		t.Skip("Skipping prebuilt ebpf conntracker related test on unsupported kernel")
	}
}

func TestEbpfConntrackerLoadTriggersOffsetGuessing(t *testing.T) {
	skipPrebuiltEbpfConntrackerTestOnUnsupportedKernel(t)

	offsetguess.TracerOffsets.Reset()

	cfg := testConfig()
	cfg.EnableRuntimeCompiler = false
	cfg.EnableCORE = false
	conntracker, err := NewEBPFConntracker(cfg)
	assert.NoError(t, err)
	require.NotNil(t, conntracker)
	t.Cleanup(conntracker.Close)

	offsets, err := offsetguess.TracerOffsets.Offsets(cfg)
	require.NoError(t, err)
	require.NotEmpty(t, offsets)
}

func TestEbpfConntrackerSkipsLoadOnOlderKernels(t *testing.T) {
	if ebpfPrebuiltConntrackerSupportedOnKernelT(t) {
		t.Skip("This test should only run on pre-4.14 kernels without backported eBPF support, like RHEL/CentOS")
	}

	offsetguess.TracerOffsets.Reset()

	cfg := testConfig()
	cfg.EnableRuntimeCompiler = false
	cfg.EnableCORE = false
	conntracker, err := NewEBPFConntracker(cfg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errPrebuiltConntrackerUnsupported)
	require.Nil(t, conntracker)
}

func TestCOREEbpfConntrackerSkipsLoadOnOlderKernels(t *testing.T) {
	if ebpfCOREConntrackerSupportedOnKernelT(t) {
		t.Skip("This test should only run on pre-4.14 kernels without backported eBPF support, like RHEL/CentOS")
	}

	offsetguess.TracerOffsets.Reset()

	cfg := testConfig()
	cfg.EnableRuntimeCompiler = false
	cfg.EnableCORE = true
	cfg.AllowPrecompiledFallback = false
	conntracker, err := NewEBPFConntracker(cfg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errCOREConntrackerUnsupported)
	require.Nil(t, conntracker)
}
