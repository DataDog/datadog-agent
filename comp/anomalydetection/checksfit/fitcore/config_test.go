// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build linux || (darwin && cgo)

package fitcore

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestInvalidConfigurationIsRejectedBeforeResources(t *testing.T) {
	for _, capacity := range []int{0, 8, 17, MaxRingCapacity + 8} {
		config := NewConsumerConfig(UnixEndpoint("/unused"))
		config.RingCapacity = capacity
		if _, err := config.validate(); err == nil {
			t.Fatalf("invalid capacity accepted: %d", capacity)
		}
	}
	for _, timeout := range []time.Duration{0, -time.Second, 1 << 62} {
		config := NewProducerConfig(UnixEndpoint("/unused"))
		config.SetupTimeout = timeout
		if _, err := config.validate(); err == nil {
			t.Fatalf("invalid timeout accepted: %v", timeout)
		}
	}
	config := NewConsumerConfig(UnixEndpoint("/unused"))
	if _, err := config.validate(); err != nil {
		t.Fatalf("default configuration rejected: %v", err)
	}
}

func TestSetupEndpointParsesUnixAndLoopbackTCPForms(t *testing.T) {
	endpoint, err := ParseEndpoint("unix:/tmp/fit.sock")
	if err != nil || endpoint != UnixEndpoint("/tmp/fit.sock") {
		t.Fatalf("unix parse = %v, %v", endpoint, err)
	}
	endpoint, err = ParseEndpoint("/tmp/fit.sock")
	if err != nil || endpoint != UnixEndpoint("/tmp/fit.sock") {
		t.Fatalf("bare path parse = %v, %v", endpoint, err)
	}
	addr := netip.MustParseAddrPort("127.0.0.1:5101")
	endpoint, err = ParseEndpoint("tcp:127.0.0.1:5101")
	if err != nil || endpoint != TCPEndpoint(addr) {
		t.Fatalf("tcp parse = %v, %v", endpoint, err)
	}
	if _, err := ParseEndpoint("tcp:0.0.0.0:5101"); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("non-loopback accepted: %v", err)
	}
	if _, err := ParseEndpoint("tcp:localhost:5101"); err == nil || !strings.Contains(err.Error(), "invalid TCP") {
		t.Fatalf("hostname accepted: %v", err)
	}
	if _, err := ParseEndpoint("tcp:[::1]:5101"); err != nil {
		t.Fatalf("IPv6 loopback rejected: %v", err)
	}
}

func TestTCPEndpointRejectsNonLoopbackDirectly(t *testing.T) {
	nonLoopback := netip.MustParseAddrPort("192.0.2.1:5101")
	err := ensureLoopback(nonLoopback)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("non-loopback accepted: %v", err)
	}
}
