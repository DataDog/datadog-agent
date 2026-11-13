// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package connection

import (
	"fmt"
	"testing"

	"github.com/google/gopacket/afpacket"
)

// TestEbpfLessTracerStopsOnAFPacketPollError checks that the reader stops at once when
// poll reports an error condition on the AF_PACKET socket, which retrying cannot clear.
func TestEbpfLessTracerStopsOnAFPacketPollError(t *testing.T) {
	requireReaderStopsOnError(t, fmt.Errorf("error reading packet: %w", afpacket.ErrPoll))
}
