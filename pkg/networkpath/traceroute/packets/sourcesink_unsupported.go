// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux && !windows

package packets

import (
	"fmt"

	"github.com/google/gopacket"
)

// NewSourceSink returns a Source and Sink implementation for this platform
func NewSourceSink(_ gopacket.LayerType) (SourceSinkHandle, error) {
	return SourceSinkHandle{}, fmt.Errorf("NewSourceSink: this platform is not supported")
}
