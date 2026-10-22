// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package fentry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestSelectTCPRecvMsgReturn(t *testing.T) {
	kvPre519 := kernel.VersionCode(5, 15, 0)
	kv519 := kernel.VersionCode(5, 19, 0)
	kv70 := kernel.VersionCode(7, 0, 0)

	count := func(n int) func(string) (int, error) {
		return func(name string) (int, error) {
			assert.Equal(t, "tcp_recvmsg", name)
			return n, nil
		}
	}
	lookupErr := func(string) (int, error) {
		return 0, errors.New("no BTF")
	}

	tests := []struct {
		name       string
		kv         kernel.Version
		paramCount func(string) (int, error)
		expected   string
	}{
		// BTF parameter count wins regardless of kernel version
		{"6 params, pre-5.19", kvPre519, count(6), tcpRecvMsgPre5190Return},
		{"6 params, 7.0", kv70, count(6), tcpRecvMsgPre5190Return},
		{"5 params, pre-5.19", kvPre519, count(5), tcpRecvMsgReturn},
		{"5 params, 7.0", kv70, count(5), tcpRecvMsgReturn},
		{"4 params, pre-5.19", kvPre519, count(4), tcpRecvMsgNoAddrLenReturn},
		{"4 params, 7.0", kv70, count(4), tcpRecvMsgNoAddrLenReturn},

		// lookup error falls back to kernel version
		{"lookup error, pre-5.19", kvPre519, lookupErr, tcpRecvMsgPre5190Return},
		{"lookup error, 5.19", kv519, lookupErr, tcpRecvMsgReturn},
		{"lookup error, 7.0", kv70, lookupErr, tcpRecvMsgReturn},

		// unexpected count falls back to kernel version
		{"3 params, pre-5.19", kvPre519, count(3), tcpRecvMsgPre5190Return},
		{"3 params, 5.19", kv519, count(3), tcpRecvMsgReturn},
		{"7 params, pre-5.19", kvPre519, count(7), tcpRecvMsgPre5190Return},
		{"7 params, 7.0", kv70, count(7), tcpRecvMsgReturn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, selectTCPRecvMsgReturn(tt.kv, tt.paramCount))
		})
	}
}

func TestEnabledProgramsRecvMsgExit(t *testing.T) {
	countEnabled := func(enabled map[string]struct{}, names ...string) []string {
		var found []string
		for _, n := range names {
			if _, ok := enabled[n]; ok {
				found = append(found, n)
			}
		}
		return found
	}

	t.Run("tcp", func(t *testing.T) {
		cfg := config.New()
		cfg.CollectTCPv4Conns = true
		cfg.CollectTCPv6Conns = true
		cfg.CollectUDPv4Conns = false
		cfg.CollectUDPv6Conns = false

		enabled, err := enabledPrograms(cfg, false)
		require.NoError(t, err)

		found := countEnabled(enabled, tcpRecvMsgReturn, tcpRecvMsgNoAddrLenReturn, tcpRecvMsgPre5190Return)
		assert.Len(t, found, 1, "expected exactly one tcp_recvmsg exit program, got %v", found)
		t.Logf("enabled tcp_recvmsg exit program: %v", found)
	})

	t.Run("udp", func(t *testing.T) {
		cfg := config.New()
		cfg.CollectTCPv4Conns = false
		cfg.CollectTCPv6Conns = false
		cfg.CollectUDPv4Conns = true
		cfg.CollectUDPv6Conns = true

		enabled, err := enabledPrograms(cfg, false)
		require.NoError(t, err)

		assert.Len(t, countEnabled(enabled, udpRecvMsgReturn), 1)
		assert.Len(t, countEnabled(enabled, udpv6RecvMsgReturn), 1)
		assert.Empty(t, countEnabled(enabled, tcpRecvMsgReturn, tcpRecvMsgNoAddrLenReturn, tcpRecvMsgPre5190Return))
	})
}
