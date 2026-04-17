// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package dns

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
)

const (
	// dnsBPFBufferSize is the per-interface BPF ring buffer used for DNS-only capture.
	// DNS traffic is low-volume so 1 MB is sufficient.
	dnsBPFBufferSize = 1 * 1024 * 1024
	// dnsSnapLen is the maximum bytes captured per packet for DNS.
	// Matches the standard EDNS0 buffer size (RFC 6891) and the Linux default,
	// ensuring full DNS responses including frame headers are captured.
	dnsSnapLen = 4096
)

// NewReverseDNS starts DNS traffic monitoring on macOS and returns a ReverseDNS
// implementation backed by libpcap packet capture.
func NewReverseDNS(cfg *config.Config, _ telemetry.Component) (ReverseDNS, error) {
	src, err := filter.NewLibpcapSource(
		filter.OptBPFFilter("port 53"),
		filter.OptBPFBufferSize(dnsBPFBufferSize),
		filter.OptSnapLen(dnsSnapLen),
	)
	if err != nil {
		return nil, err
	}
	return newDarwinDNSMonitorWithSource(cfg, src)
}

// newDarwinDNSMonitorWithSource constructs a DNS monitor using the provided
// PacketSource. Used by NewReverseDNS and tests that inject a mock source.
func newDarwinDNSMonitorWithSource(cfg *config.Config, src filter.PacketSource) (*socketFilterSnooper, error) {
	ethernetParser := newDNSParser(layers.LayerTypeEthernet, cfg)
	loopbackParser := newDNSParser(layers.LayerTypeLoopback, cfg)

	parserSelector := func(info filter.PacketInfo) *dnsParser {
		if info.LinkLayerType() == layers.LayerTypeLoopback {
			return loopbackParser
		}
		return ethernetParser
	}

	snoop, err := newSocketFilterSnooper(cfg, src, parserSelector)
	if err != nil {
		return nil, err
	}
	snoop.startPolling()
	return snoop, nil
}
