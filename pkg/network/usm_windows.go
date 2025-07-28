// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build npm

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

// USMProtocolsData encapsulates the protocols data for Windows version of USM.
type USMProtocolsData struct {
	HTTP map[http.Key]*http.RequestStats
}

// NewUSMProtocolsData creates a new instance of USMProtocolsData with initialized maps.
func NewUSMProtocolsData() USMProtocolsData {
	return USMProtocolsData{
		HTTP: make(map[http.Key]*http.RequestStats),
	}
}

// Reset clears the maps in USMProtocolsData.
func (o *USMProtocolsData) Reset() {
	if len(o.HTTP) > 0 {
		o.HTTP = make(map[http.Key]*http.RequestStats)
	}
}

// processUSMDelta processes the USM delta for Windows.
func (ns *networkState) processUSMDelta(stats map[protocols.ProtocolType]interface{}) {
	for protocolType, protocolStats := range stats {
		switch protocolType {
		case protocols.HTTP:
			stats := protocolStats.(map[http.Key]*http.RequestStats)
			ns.storeHTTPStats(stats)
		}
	}
}
