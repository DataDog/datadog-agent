// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// This doesn't need BPF, but it's built with this tag to only run with
// system-probe tests.
//go:build test && linux_bpf && dd_discovery_cgo

package module

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// Check that we get (only) listening processes for all expected protocols using the services endpoint.
func TestServicesBasic(t *testing.T) {
	discovery := setupDiscoveryModule(t)

	var expectedPIDs []int
	var unexpectedPIDs []int
	exceptedTCPPorts := make(map[int]int)
	exceptedUDPPorts := make(map[int]int)

	startTCP := func(proto string) {
		f, server := startTCPServer(t, proto, "")
		cmd := startProcessWithFile(t, f)
		expectedPIDs = append(expectedPIDs, cmd.Process.Pid)
		exceptedTCPPorts[cmd.Process.Pid] = server.Port

		f, _ = startTCPClient(t, proto, server)
		cmd = startProcessWithFile(t, f)
		unexpectedPIDs = append(unexpectedPIDs, cmd.Process.Pid)
	}

	startUDP := func(proto string) {
		f, server := startUDPServer(t, proto, ":8083")
		cmd := startProcessWithFile(t, f)
		expectedPIDs = append(expectedPIDs, cmd.Process.Pid)
		exceptedUDPPorts[cmd.Process.Pid] = server.Port

		f, _ = startUDPClient(t, proto, server)
		cmd = startProcessWithFile(t, f)
		unexpectedPIDs = append(unexpectedPIDs, cmd.Process.Pid)
	}

	startTCP("tcp4")
	startTCP("tcp6")
	startUDP("udp4")
	startUDP("udp6")

	seen := make(map[int]model.Service)
	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServices(collect, discovery.url)
		for _, s := range resp.Services {
			seen[s.PID] = s
		}

		for _, pid := range expectedPIDs {
			require.Contains(collect, seen, pid)
			assert.Equal(collect, seen[pid].PID, pid)
			if expectedTCPPort, ok := exceptedTCPPorts[pid]; ok {
				require.Contains(collect, seen[pid].TCPPorts, uint16(expectedTCPPort))
			}
			if expectedUDPPort, ok := exceptedUDPPorts[pid]; ok {
				require.Contains(collect, seen[pid].UDPPorts, uint16(expectedUDPPort))
			}
		}
		for _, pid := range unexpectedPIDs {
			assert.NotContains(collect, seen, pid)
		}
	}, 30*time.Second, 100*time.Millisecond)
}
