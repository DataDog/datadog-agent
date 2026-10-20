// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package procnetsocket

import (
	"context"
	"strings"
	"testing"
)

func TestVulnHuntSubsystemPlatformDivergence_NonLinuxReadersDoNotOpenProc(t *testing.T) {
	readers := []struct {
		name string
		fn   func(context.Context, string) ([]SocketEntry, error)
	}{
		{"tcp4", ReadTCP4},
		{"tcp6", ReadTCP6},
		{"udp4", ReadUDP4},
		{"udp6", ReadUDP6},
		{"unix", ReadUnix},
	}

	for _, reader := range readers {
		t.Run(reader.name, func(t *testing.T) {
			_, err := reader.fn(context.Background(), DefaultProcPath)
			if err == nil {
				t.Fatalf("expected non-Linux platform error")
			}
			if !strings.Contains(err.Error(), "not supported on this platform") {
				t.Fatalf("expected platform error, got %v", err)
			}
			if strings.Contains(err.Error(), "open ") {
				t.Fatalf("non-Linux reader attempted direct open: %v", err)
			}
		})
	}
}
