// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestSocketCaptureTrigger_RealSystemProbe exercises socketCaptureTrigger
// against an already-running system-probe packet_capture module instead of
// the fake server used by the other tests in this file. It is opt-in (not
// run by default, including in CI) because it requires a live system-probe
// process with the packet_capture module loaded, root/eBPF privileges, and
// real network traffic to observe.
//
// Enable with:
//
//	PCAP_REAL_SYSPROBE_TEST=1 \
//	PCAP_REAL_SYSPROBE_SOCKET=/opt/datadog-agent/run/sysprobe.sock \
//	PCAP_REAL_SYSPROBE_IFACE=eth0 \
//	dda inv test --targets=./pkg/privateactionrunner/bundles/remoteaction/pcap/... --build-include=<tags incl. pcap>
//
// while generating traffic on the target interface (e.g. `ping 8.8.8.8`) for
// at least the capture duration below.
func TestSocketCaptureTrigger_RealSystemProbe(t *testing.T) {
	if os.Getenv("PCAP_REAL_SYSPROBE_TEST") != "1" {
		t.Skip("set PCAP_REAL_SYSPROBE_TEST=1 to run against a live system-probe")
	}

	socketPath := os.Getenv("PCAP_REAL_SYSPROBE_SOCKET")
	if socketPath == "" {
		socketPath = "/opt/datadog-agent/run/sysprobe.sock"
	}
	iface := os.Getenv("PCAP_REAL_SYSPROBE_IFACE")
	if iface == "" {
		iface = "eth0"
	}

	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetInTest("system_probe_config.sysprobe_socket", socketPath)

	trigger := &socketCaptureTrigger{}
	packetCount, fileSizeBytes, actualDuration, pcapPath, err := trigger.Capture(context.Background(), RunCaptureInputs{
		BPFFilter:    "icmp",
		DurationSecs: 3,
		Interface:    iface,
	})
	require.NoError(t, err)
	defer os.Remove(pcapPath)

	t.Logf("captured %d packets (%d bytes) in %s -> %s", packetCount, fileSizeBytes, actualDuration, pcapPath)

	assert.Greater(t, packetCount, 0, "expected at least one ICMP packet — is traffic being generated on %s during the capture window?", iface)
	assert.Greater(t, fileSizeBytes, int64(0))

	data, err := os.ReadFile(pcapPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(data), 24, "pcap file must contain at least the global header")
	assert.Equal(t, []byte{0xd4, 0xc3, 0xb2, 0xa1}, data[0:4], "pcap global header magic number mismatch")
}
