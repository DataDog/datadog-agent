// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"encoding/binary"
	"hash/fnv"
	"net/netip"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// NetworkPathCollectorMetricPrefix is a metric prefix for network path collector
const NetworkPathCollectorMetricPrefix = "datadog.network_path.collector."

// PathtestMetadata contains metadata used to annotate the result of a traceroute.
// This data is not used by the traceroute itself.
type PathtestMetadata struct {
	// ReverseDNSHostname is an optional hostname which will be used in place of rDNS querying for
	// the destination address.
	ReverseDNSHostname string
	// Namespaces is the set of NetFlow namespaces that observed this destination.
	// Empty for CNM-origin tests.
	Namespaces []string
	// ExporterAddrs is the set of NetFlow exporter device IPs that observed this destination.
	// Empty for CNM-origin tests.
	ExporterAddrs []netip.Addr
}

// Pathtest details of information necessary to run a traceroute
type Pathtest struct {
	Hostname          string
	Port              uint16
	Protocol          payload.Protocol
	SourceContainerID string
	// Origin identifies how this path test was triggered.
	Origin model.OriginType
	// DestIP is the originally-observed destination IP, preserved for event tagging
	// even when Hostname is a domain name. Not included in the dedup hash.
	DestIP   netip.Addr
	Metadata PathtestMetadata
}

// GetHash returns the hash of the Pathtest
func (p Pathtest) GetHash() uint64 {
	h := fnv.New64()
	_, _ = h.Write([]byte(p.Hostname))
	_ = binary.Write(h, binary.LittleEndian, p.Port)
	_, _ = h.Write([]byte(p.Protocol))
	_, _ = h.Write([]byte(p.SourceContainerID))
	_, _ = h.Write([]byte(p.Origin))
	return h.Sum64()
}
