// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package fentry

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddbtf "github.com/DataDog/datadog-agent/pkg/ebpf/btf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
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
	if _, err := ddbtf.GetKernelSpec(); err != nil {
		t.Skipf("no usable kernel BTF, fentry is unsupported: %s", err)
	}

	cfg := config.New()
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true
	cfg.CollectUDPv4Conns = false
	cfg.CollectUDPv6Conns = false

	n, err := ddbtf.GetKernelFuncArgCount("tcp_recvmsg")
	require.NoError(t, err)
	expected, err := selectTCPRecvMsgReturn(n)
	require.NoError(t, err)

	enabled, err := enabledPrograms(cfg, false)
	require.NoError(t, err)

	found := filterEnabled(enabled, tcpRecvMsgReturn, tcpRecvMsgNoAddrLenReturn, tcpRecvMsgPre5190Return)
	assert.Equal(t, []string{expected}, found, "tcp_recvmsg has %d parameters", n)
	t.Logf("tcp_recvmsg has %d parameters, enabled exit program: %v", n, found)
}
