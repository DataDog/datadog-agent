// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package fentry

import (
	"fmt"
	"syscall"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ddbtf "github.com/DataDog/datadog-agent/pkg/ebpf/btf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ddfeatures "github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
)

func TestSelectTCPRecvMsgReturn(t *testing.T) {
	tests := []struct {
		paramCount int
		expected   string
	}{
		{6, tcpRecvMsgPre5190Return},
		{5, tcpRecvMsgReturn},
		{4, tcpRecvMsgNoAddrLenReturn},
		{3, ""},
		{7, ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d params", tt.paramCount), func(t *testing.T) {
			probe, err := selectTCPRecvMsgReturn(tt.paramCount)
			if tt.expected == "" {
				require.ErrorIs(t, err, ErrorUnsupported)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, probe)
		})
	}
}

func TestEnabledProgramsTCPRecvMsgExit(t *testing.T) {
	filterEnabled := func(enabled map[string]struct{}, names ...string) []string {
		var found []string
		for _, n := range names {
			if _, ok := enabled[n]; ok {
				found = append(found, n)
			}
		}
		return found
	}

	// Only skip kernels without usable BTF (e.g. CentOS 7): fentry can't run
	// there, so production never reaches enabledPrograms. Availability is
	// checked separately so that errors from the tcp_recvmsg lookup itself still
	// fail the test.
	//
	// This deliberately doesn't gate on ebpftest.SupportedBuildModes() including
	// Fentry: that also excludes BTF kernels with the fentry detach deadlock
	// (most 5.4-5.15 KMT kernels), which are the only CI kernels with the 6-arg
	// pre-5.19 tcp_recvmsg. enabledPrograms only reads BTF and the kernel version,
	// it doesn't load or attach anything, so it is safe to run there.
	spec, err := ddbtf.GetKernelSpec()
	if err != nil {
		t.Skipf("no usable kernel BTF, fentry is unsupported: %s", err)
	}

	cfg := config.New()
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true
	cfg.CollectUDPv4Conns = false
	cfg.CollectUDPv6Conns = false

	n, err := ddbtf.GetKernelFuncArgCount(spec, "tcp_recvmsg")
	require.NoError(t, err)
	expected, err := selectTCPRecvMsgReturn(n)
	require.NoError(t, err)

	enabled, err := enabledPrograms(cfg, false)
	require.NoError(t, err)

	found := filterEnabled(enabled, tcpRecvMsgReturn, tcpRecvMsgNoAddrLenReturn, tcpRecvMsgPre5190Return)
	assert.Equal(t, []string{expected}, found, "tcp_recvmsg has %d parameters", n)
	t.Logf("tcp_recvmsg has %d parameters, enabled exit program: %v", n, found)
}

func TestLoadTracerRetriesWithoutTPBTF(t *testing.T) {
	if err := ddfeatures.SupportsFentry("tcp_recvmsg"); err != nil {
		t.Skipf("fentry not supported: %s", err)
	}
	prevInit := fentryTracerInit
	t.Cleanup(func() {
		fentryTracerInit = prevInit
		util.NetDevQueueTPBTFRejected.Store(false)
	})
	util.NetDevQueueTPBTFRejected.Store(false)

	var managers []*ddebpf.Manager
	var rejectedOnRetry bool
	fentryTracerInit = func(_ bytecode.AssetReader, _ manager.Options, _ *config.Config, m *ddebpf.Manager) (func(), error) {
		managers = append(managers, m)
		if len(managers) == 1 {
			return nil, fmt.Errorf("failed to init ebpf manager: %w",
				fmt.Errorf("program %s: load program: %w", netDevQueueTPBTF, &ebpf.VerifierError{Cause: syscall.EACCES}))
		}
		rejectedOnRetry = util.NetDevQueueTPBTFRejected.Load()
		return nil, nil
	}

	cfg := config.New()
	cfg.EnableFentry = true
	m, _, err := LoadTracer(cfg, manager.Options{}, nil)
	require.NoError(t, err)
	require.Len(t, managers, 2)
	assert.True(t, rejectedOnRetry)
	assert.NotSame(t, managers[0], managers[1], "the retry uses a new manager")
	assert.Same(t, managers[1], m)
	// a recovered rejection is not a CO-RE verifier failure
	assert.NotEqual(t, int32(ddebpf.VerifierError), ddebpf.GetCORETelemetryByAsset()["tracer-fentry"])
}

func TestEnabledProgramsSkipsRejectedTPBTF(t *testing.T) {
	if _, err := ddbtf.GetKernelSpec(); err != nil {
		t.Skipf("no usable kernel BTF, fentry is unsupported: %s", err)
	}
	prev := netDevQueueTPBTFSupported
	t.Cleanup(func() {
		netDevQueueTPBTFSupported = prev
		util.NetDevQueueTPBTFRejected.Store(false)
	})
	netDevQueueTPBTFSupported = func() bool { return true }

	cfg := config.New()
	cfg.CollectUDPv4Conns = false
	cfg.CollectUDPv6Conns = false
	for _, rejected := range []bool{false, true} {
		util.NetDevQueueTPBTFRejected.Store(rejected)
		enabled, err := enabledPrograms(cfg, true)
		require.NoError(t, err)
		_, tpBTF := enabled[netDevQueueTPBTF]
		_, rawTP := enabled[netDevQueueRawTracepoint]
		assert.Equal(t, !rejected, tpBTF, "rejected=%v", rejected)
		assert.Equal(t, rejected, rawTP, "rejected=%v", rejected)
	}
}
