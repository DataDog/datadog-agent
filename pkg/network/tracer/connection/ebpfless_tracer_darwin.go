// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package connection

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
)

const (
	// the segment length to read on Darwin
	// For on Darwin: ethernet + ip header + tcp header (no ethernet)
	segmentLen = 18 + 20 + 20
)

// createPacketSource creates a Darwin-specific libpcap packet source
func createPacketSource(_ *config.Config) (filter.PacketSource, error) {
	packetSrc, err := filter.NewLibpcapSource(
		filter.OptSnapLen(segmentLen))
	if err != nil {
		return nil, fmt.Errorf("error creating libpcap source: %w", err)
	}
	return packetSrc, nil
}
