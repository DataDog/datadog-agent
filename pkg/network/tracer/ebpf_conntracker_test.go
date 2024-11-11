// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
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
	conntracker, err := NewEBPFConntracker(cfg, nil)
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
	conntracker, err := NewEBPFConntracker(cfg, nil)
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
	cfg.AllowPrebuiltFallback = false
	conntracker, err := NewEBPFConntracker(cfg, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errCOREConntrackerUnsupported)
	require.Nil(t, conntracker)
}

func TestEbpfConntrackerEnsureMapType(t *testing.T) {
	if !ebpfPrebuiltConntrackerSupportedOnKernelT(t) && !ebpfCOREConntrackerSupportedOnKernelT(t) {
		t.Skip("ebpf conntracker not supported on this kernel")
	}

	haveLRUHash := features.HaveMapType(ebpf.LRUHash) == nil

	checkMap := func(t *testing.T, cfg *config.Config) {
		conntracker, err := NewEBPFConntracker(cfg, nil)
		t.Cleanup(conntracker.Close)
		require.NoError(t, err, "error creating ebpf conntracker")
		m, _, err := conntracker.(*ebpfConntracker).m.GetMap(probes.ConntrackMap)
		require.NoError(t, err, "error getting conntrack map")
		require.NotNil(t, m, "conntrack map is nil")
		if haveLRUHash {
			assert.Equal(t, m.Type(), ebpf.LRUHash)
		} else {
			assert.Equal(t, m.Type(), ebpf.Hash)
		}
	}

	t.Run("runtime-compiled", func(t *testing.T) {
		cfg := testConfig()
		cfg.EnableRuntimeCompiler = true
		cfg.EnableCORE = false
		cfg.AllowPrebuiltFallback = false
		checkMap(t, cfg)
	})

	t.Run("prebuilt", func(t *testing.T) {
		cfg := testConfig()
		cfg.EnableRuntimeCompiler = false
		cfg.EnableCORE = false
		checkMap(t, cfg)
	})

	t.Run("co-re", func(t *testing.T) {
		cfg := testConfig()
		cfg.EnableRuntimeCompiler = false
		cfg.EnableCORE = true
		cfg.AllowPrebuiltFallback = false
		checkMap(t, cfg)
	})
}
