// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || unix

package packets

import (
	"fmt"

	"github.com/google/gopacket"
)

// NewSourceSink returns a Source and Sink implementation for this platform
func NewSourceSink(family gopacket.LayerType) (SourceSinkHandle, error) {
	sink, err := NewSinkUnix(family)
	if err != nil {
		return SourceSinkHandle{}, fmt.Errorf("NewSourceSink failed to make SinkUnix: %w", err)
	}

	source, err := NewAFPacketSource()
	if err != nil {
		sink.Close()
		return SourceSinkHandle{}, fmt.Errorf("NewSourceSink failed to make AFPacketSource: %w", err)
	}

	return SourceSinkHandle{
		Source:        source,
		Sink:          sink,
		MustClosePort: false,
	}, nil
}
