// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows && npm

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

func (ns *networkState) processUSMDelta(stats map[protocols.ProtocolType]interface{}) {
	for protocolType, protocolStats := range stats {
		switch protocolType {
		case protocols.HTTP:
			stats := protocolStats.(map[http.Key]*http.RequestStats)
			ns.storeHTTPStats(stats)
		}
	}
}
