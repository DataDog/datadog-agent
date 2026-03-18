// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package dns

import (
	"time"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
)

// dnsMonitor implements ReverseDNS for macOS using libpcap packet capture.
// It embeds socketFilterSnooper and overrides packet processing to dispatch
// between ethernet and BSD loopback parsers based on per-packet layer type.
type dnsMonitor struct {
	*socketFilterSnooper
	ethernetParser *dnsParser
	loopbackParser *dnsParser
}

var _ ReverseDNS = &dnsMonitor{}

// NewReverseDNS starts DNS traffic monitoring on macOS and returns a ReverseDNS
// implementation backed by libpcap packet capture.
func NewReverseDNS(cfg *config.Config, _ telemetry.Component) (ReverseDNS, error) {
	src, err := filter.NewSubSource(cfg, filter.IsDNSPacket)
	if err != nil {
		return nil, err
	}
	return newDarwinDNSMonitorWithSource(cfg, src)
}

// newDarwinDNSMonitorWithSource constructs a dnsMonitor using the provided
// PacketSource. Used by NewReverseDNS and tests that inject a mock source.
func newDarwinDNSMonitorWithSource(cfg *config.Config, src filter.PacketSource) (*dnsMonitor, error) {
	m := &dnsMonitor{
		ethernetParser: newDNSParser(layers.LayerTypeEthernet, cfg),
		loopbackParser: newDNSParser(layers.LayerTypeLoopback, cfg),
	}
	snoop, err := newSocketFilterSnooper(cfg, src, m.processPacket)
	if err != nil {
		return nil, err
	}
	m.socketFilterSnooper = snoop
	return m, nil
}

// processPacket selects the appropriate parser based on the packet's link-layer
// type, then delegates to the embedded snooper's cache, stat keeper, and telemetry.
func (m *dnsMonitor) processPacket(data []byte, info filter.PacketInfo, ts time.Time) error {
	pktInfo, _ := info.(*filter.DarwinPacketInfo)
	if pktInfo == nil {
		pktInfo = &filter.DarwinPacketInfo{}
	}

	var parser *dnsParser
	if pktInfo.LayerType == layers.LayerTypeLoopback {
		parser = m.loopbackParser
	} else {
		parser = m.ethernetParser
	}

	t := m.getCachedTranslation()
	dnsInfo := dnsPacketInfo{}

	if err := parser.ParseInto(data, t, &dnsInfo); err != nil {
		switch err {
		case errSkippedPayload:
		case errTruncated:
			snooperTelemetry.truncatedPkts.Inc()
		default:
			snooperTelemetry.decodingErrors.Inc()
		}
		return nil
	}

	m.processPacketInfo(dnsInfo, ts)

	if dnsInfo.pktType == successfulResponse {
		m.addToCache(t)
		snooperTelemetry.successes.Inc()
	} else if dnsInfo.pktType == failedResponse {
		snooperTelemetry.errors.Inc()
	} else {
		snooperTelemetry.queries.Inc()
	}

	return nil
}
