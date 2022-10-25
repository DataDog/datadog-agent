// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// KeyTuple represents the network tuple for a group of HTTP transactions
type KeyTuple struct {
	SrcIPHigh uint64
	SrcIPLow  uint64

	DstIPHigh uint64
	DstIPLow  uint64

	// ports separated for alignment/size optimization
	SrcPort uint16
	DstPort uint16
}

// Key is an identifier for a group of HTTP transactions
type Key struct {
	// this field order is intentional to help the GC pointer tracking
	Path Path
	KeyTuple
	Method Method
}

// Path represents the HTTP path
type Path struct {
	Content  string
	FullPath bool
}

// NewKey generates a new Key
func NewKey(saddr, daddr util.Address, sport, dport uint16, path string, fullPath bool, method Method) Key {
	return Key{
		KeyTuple: NewKeyTuple(saddr, daddr, sport, dport),
		Path: Path{
			Content:  path,
			FullPath: fullPath,
		},
		Method: method,
	}
}

// NewKeyTuple generates a new KeyTuple
func NewKeyTuple(saddr, daddr util.Address, sport, dport uint16) KeyTuple {
	saddrl, saddrh := util.ToLowHigh(saddr)
	daddrl, daddrh := util.ToLowHigh(daddr)
	return KeyTuple{
		SrcIPHigh: saddrh,
		SrcIPLow:  saddrl,
		SrcPort:   sport,
		DstIPHigh: daddrh,
		DstIPLow:  daddrl,
		DstPort:   dport,
	}
}
