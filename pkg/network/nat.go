// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// GetNATLocalAddress returns the translated (local ip, local port) pair
func GetNATLocalAddress(c ConnectionStats) (util.Address, uint16) {
	localIP := c.Source
	localPort := c.SPort

	if c.IPTranslation != nil && !c.IPTranslation.ReplDstIP.IsZero() {
		// Fields are flipped
		localIP = c.IPTranslation.ReplDstIP
		localPort = c.IPTranslation.ReplDstPort
	}
	return localIP, localPort
}

// GetNATRemoteAddress returns the translated (remote ip, remote port) pair
func GetNATRemoteAddress(c ConnectionStats) (util.Address, uint16) {
	remoteIP := c.Dest
	remotePort := c.DPort

	if c.IPTranslation != nil && !c.IPTranslation.ReplSrcIP.IsZero() {
		// Fields are flipped
		remoteIP = c.IPTranslation.ReplSrcIP
		remotePort = c.IPTranslation.ReplSrcPort
	}
	return remoteIP, remotePort
}
