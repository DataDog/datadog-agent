// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"encoding/binary"
	"hash/fnv"
)

// Pathtest details of information necessary to run a traceroute (pathtrace)
type Pathtest struct {
	Hostname string
	Port     uint16
	Protocol string
}

// GetHash returns the hash of the Pathtest
func (p Pathtest) GetHash() uint64 {
	h := fnv.New64()
	h.Write([]byte(p.Hostname))                  //nolint:errcheck
	binary.Write(h, binary.LittleEndian, p.Port) //nolint:errcheck
	h.Write([]byte(p.Protocol))
	return h.Sum64()
}
