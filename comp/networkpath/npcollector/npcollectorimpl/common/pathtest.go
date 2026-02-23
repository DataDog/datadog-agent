// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"encoding/binary"
	"hash/fnv"

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
}

// Pathtest details of information necessary to run a traceroute
type Pathtest struct {
	Hostname          string
	Port              uint16
	Protocol          payload.Protocol
	SourceContainerID string
	Metadata          PathtestMetadata
}

// GetHash returns the hash of the Pathtest
func (p Pathtest) GetHash() uint64 {
	h := fnv.New64()
	_, _ = h.Write([]byte(p.Hostname))
	_ = binary.Write(h, binary.LittleEndian, p.Port)
	_, _ = h.Write([]byte(p.Protocol))
	_, _ = h.Write([]byte(p.SourceContainerID))
	return h.Sum64()
}
