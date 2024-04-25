// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npschedulerimpl

import (
	"encoding/binary"
	"hash/fnv"
)

type pathtest struct {
	hostname string
	port     uint16
}

func (p pathtest) getHash() uint64 {
	// TODO: TESTME
	h := fnv.New64()
	h.Write([]byte(p.hostname))                  //nolint:errcheck
	binary.Write(h, binary.LittleEndian, p.port) //nolint:errcheck
	return h.Sum64()
}
