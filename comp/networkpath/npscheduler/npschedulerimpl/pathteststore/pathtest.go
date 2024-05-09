// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package pathteststore

import (
	"encoding/binary"
	"hash/fnv"
)

type Pathtest struct {
	Hostname string
	Port     uint16
}

func (p Pathtest) getHash() uint64 {
	// TODO: TESTME
	h := fnv.New64()
	h.Write([]byte(p.Hostname))                  //nolint:errcheck
	binary.Write(h, binary.LittleEndian, p.Port) //nolint:errcheck
	return h.Sum64()
}
